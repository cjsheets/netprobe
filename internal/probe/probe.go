package probe

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

type Prober interface {
	Probe(context.Context, model.Target) model.Observation
}
type Native struct{}

func (Native) Probe(ctx context.Context, t model.Target) model.Observation {
	o := model.Observation{Timestamp: time.Now().UTC(), Target: t.Name, ProbeType: t.Type}
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	var latency time.Duration
	var dst, src, iface, route string
	var err error
	switch t.Type {
	case model.ICMP:
		latency, dst, src, err = icmpProbe(ctx, t)
	case model.TCP:
		latency, dst, src, err = tcpProbe(ctx, t)
	case model.DNS:
		latency, dst, src, err = dnsProbe(ctx, t)
	case model.HTTP:
		latency, dst, src, err = httpProbe(ctx, t)
	default:
		err = fmt.Errorf("unsupported probe type %q", t.Type)
	}
	if src != "" {
		iface = interfaceForIP(src)
	}
	if dst != "" && src != "" {
		route = src + " -> " + dst
	}
	o.Destination, o.Source, o.Interface, o.Route = dst, src, iface, route
	if err == nil {
		ms := float64(latency.Microseconds()) / 1000
		o.Success = true
		o.LatencyMS = &ms
		return o
	}
	o.ErrorCategory = category(err)
	o.ErrorMessage = safeError(err)
	return o
}

func localAddr(t model.Target, network string) (net.Addr, error) {
	source, err := effectiveSource(t)
	if err != nil {
		return nil, err
	}
	if source == "" {
		return nil, nil
	}
	ip := net.ParseIP(source)
	if ip == nil {
		return nil, fmt.Errorf("invalid source address")
	}
	if strings.HasPrefix(network, "tcp") {
		return &net.TCPAddr{IP: ip}, nil
	}
	return &net.UDPAddr{IP: ip}, nil
}

func effectiveSource(t model.Target) (string, error) {
	if t.Source != "" {
		return t.Source, nil
	}
	if t.Interface == "" {
		return "", nil
	}
	in, err := net.InterfaceByName(t.Interface)
	if err != nil {
		return "", fmt.Errorf("interface %q is unavailable: %w", t.Interface, err)
	}
	addrs, err := in.Addrs()
	if err != nil {
		return "", fmt.Errorf("read interface %q addresses: %w", t.Interface, err)
	}
	for _, a := range addrs {
		p, e := netip.ParsePrefix(a.String())
		if e == nil && p.Addr().Is4() {
			return p.Addr().String(), nil
		}
	}
	for _, a := range addrs {
		p, e := netip.ParsePrefix(a.String())
		if e == nil {
			return p.Addr().String(), nil
		}
	}
	return "", fmt.Errorf("interface %q has no usable IP address", t.Interface)
}

func selectedSource(destination, explicit string) string {
	if explicit != "" {
		return explicit
	}
	c, err := net.Dial("udp", net.JoinHostPort(destination, "9"))
	if err != nil {
		return ""
	}
	defer c.Close()
	return addrIP(c.LocalAddr())
}
func tcpProbe(ctx context.Context, t model.Target) (time.Duration, string, string, error) {
	addr := net.JoinHostPort(t.Host, fmt.Sprint(t.Port))
	la, e := localAddr(t, "tcp")
	if e != nil {
		return 0, "", "", e
	}
	d := net.Dialer{LocalAddr: la}
	start := time.Now()
	c, e := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)
	if e != nil {
		return elapsed, resolved(t.Host), addrIP(la), e
	}
	defer c.Close()
	return elapsed, addrIP(c.RemoteAddr()), addrIP(c.LocalAddr()), nil
}
func dnsProbe(ctx context.Context, t model.Target) (time.Duration, string, string, error) {
	resolver := t.Resolver
	if _, _, e := net.SplitHostPort(resolver); e != nil {
		resolver = net.JoinHostPort(resolver, "53")
	}
	var src, destination string
	r := net.Resolver{PreferGo: true, Dial: func(c context.Context, network, _ string) (net.Conn, error) {
		la, e := localAddr(t, "udp")
		if e != nil {
			return nil, e
		}
		d := net.Dialer{LocalAddr: la}
		conn, e := d.DialContext(c, "udp", resolver)
		if e == nil {
			src = addrIP(conn.LocalAddr())
			destination = addrIP(conn.RemoteAddr())
		}
		return conn, e
	}}
	start := time.Now()
	ips, e := r.LookupHost(ctx, t.Query)
	elapsed := time.Since(start)
	dst, _, _ := net.SplitHostPort(resolver)
	if destination != "" {
		dst = destination
	}
	if e != nil {
		return elapsed, dst, src, e
	}
	if len(ips) == 0 {
		return elapsed, dst, src, errors.New("DNS returned no addresses")
	}
	return elapsed, dst, src, nil
}
func httpProbe(ctx context.Context, t model.Target) (time.Duration, string, string, error) {
	u, _ := url.Parse(t.URL)
	var dst, src string
	dialer := net.Dialer{}
	if t.Source != "" {
		dialer.LocalAddr, _ = localAddr(t, "tcp")
	}
	tr := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: func(c context.Context, n, a string) (net.Conn, error) {
		conn, e := dialer.DialContext(c, n, a)
		if e == nil {
			dst = addrIP(conn.RemoteAddr())
			src = addrIP(conn.LocalAddr())
		}
		return conn, e
	}, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", "netprobe/1")
	start := time.Now()
	resp, e := client.Do(req)
	elapsed := time.Since(start)
	if e != nil {
		return elapsed, dst, src, e
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return elapsed, dst, src, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	return elapsed, dst, src, nil
}
func resolved(host string) string {
	ips, e := net.LookupIP(host)
	if e == nil && len(ips) > 0 {
		return ips[0].String()
	}
	return host
}
func addrIP(a net.Addr) string {
	if a == nil {
		return ""
	}
	s := a.String()
	h, _, e := net.SplitHostPort(s)
	if e == nil {
		return h
	}
	return s
}
func interfaceForIP(s string) string {
	ip := net.ParseIP(s)
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			p, e := netip.ParsePrefix(a.String())
			if e == nil && p.Addr().Unmap().String() == ip.String() {
				return in.Name
			}
		}
	}
	return ""
}
func category(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	var de *net.DNSError
	if errors.As(err, &de) {
		return "dns"
	}
	var oe *net.OpError
	if errors.As(err, &oe) {
		if errors.Is(oe.Err, syscall.EACCES) || errors.Is(oe.Err, syscall.EPERM) {
			return "permission"
		}
		return "network"
	}
	if strings.HasPrefix(err.Error(), "HTTP status") {
		return "http_status"
	}
	return "probe"
}
func sanitize(s string) string {
	if len(s) > 240 {
		return s[:240]
	}
	return s
}

func safeError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return sanitize(ue.Err.Error())
	}
	return sanitize(err.Error())
}
