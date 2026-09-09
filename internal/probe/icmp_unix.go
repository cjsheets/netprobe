//go:build !windows

package probe

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

func icmpProbe(ctx context.Context, t model.Target) (time.Duration, string, string, error) {
	ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", t.Host)
	if e != nil || len(ips) == 0 {
		return 0, "", "", fmt.Errorf("resolve ICMP target: %w", e)
	}
	dst := ips[0]
	bind := "0.0.0.0"
	if source, sourceErr := effectiveSource(t); sourceErr != nil {
		return 0, dst.String(), "", sourceErr
	} else if source != "" {
		bind = source
	}
	c, e := icmp.ListenPacket("udp4", bind)
	if e != nil {
		return 0, dst.String(), "", fmt.Errorf("ICMP permission unavailable: %w; on Linux set net.ipv4.ping_group_range for this user, or grant only cap_net_raw to netprobe", e)
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	id := os.Getpid() & 0xffff
	msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Code: 0, Body: &icmp.Echo{ID: id, Seq: int(time.Now().UnixNano() & 0xffff), Data: []byte("netprobe")}}
	b, _ := msg.Marshal(nil)
	start := time.Now()
	if _, e = c.WriteTo(b, &net.UDPAddr{IP: dst}); e != nil {
		return time.Since(start), dst.String(), bind, e
	}
	buf := make([]byte, 1500)
	for {
		n, peer, e := c.ReadFrom(buf)
		if e != nil {
			return time.Since(start), dst.String(), bind, e
		}
		m, e := icmp.ParseMessage(1, buf[:n])
		if e == nil && m.Type == ipv4.ICMPTypeEchoReply {
			_ = peer
			return time.Since(start), dst.String(), selectedSource(dst.String(), bindIfSpecific(bind)), nil
		}
	}
}

func bindIfSpecific(s string) string {
	if s == "0.0.0.0" {
		return ""
	}
	return s
}
