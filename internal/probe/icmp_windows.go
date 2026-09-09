//go:build windows

package probe

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"

	"github.com/chadsheets/netprobe/internal/model"
	"golang.org/x/sys/windows"
)

var iphlp = windows.NewLazySystemDLL("iphlpapi.dll")
var icmpCreate = iphlp.NewProc("IcmpCreateFile")
var icmpClose = iphlp.NewProc("IcmpCloseHandle")
var icmpSend = iphlp.NewProc("IcmpSendEcho")
var icmpSendEx = iphlp.NewProc("IcmpSendEcho2Ex")

type icmpEchoReply struct {
	Address, Status, RoundTripTime uint32
	DataSize, Reserved             uint16
	Data                           uintptr
	Options                        [8]byte
}

func icmpProbe(ctx context.Context, t model.Target) (time.Duration, string, string, error) {
	ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", t.Host)
	if e != nil || len(ips) == 0 {
		return 0, "", "", fmt.Errorf("resolve ICMP target: %w", e)
	}
	ip := ips[0].To4()
	h, _, e := icmpCreate.Call()
	if h == uintptr(windows.InvalidHandle) || h == 0 {
		return 0, ip.String(), "", fmt.Errorf("Windows IcmpCreateFile: %w", e)
	}
	defer icmpClose.Call(h)
	payload := []byte("netprobe")
	reply := make([]byte, int(unsafe.Sizeof(icmpEchoReply{}))+len(payload)+16)
	timeout := uint32(t.Timeout / time.Millisecond)
	start := time.Now()
	destination := uintptr(binary.LittleEndian.Uint32(ip))
	sourceText, sourceErr := effectiveSource(t)
	if sourceErr != nil {
		return 0, ip.String(), "", sourceErr
	}
	var r uintptr
	var callErr error
	if sourceText != "" {
		sourceIP := net.ParseIP(sourceText).To4()
		if sourceIP == nil {
			return 0, ip.String(), "", fmt.Errorf("Windows native ICMP requires an IPv4 source address")
		}
		r, _, callErr = icmpSendEx.Call(h, 0, 0, 0, uintptr(binary.LittleEndian.Uint32(sourceIP)), destination, uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), 0, uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout))
	} else {
		r, _, callErr = icmpSend.Call(h, destination, uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), 0, uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout))
	}
	elapsed := time.Since(start)
	if r == 0 {
		return elapsed, ip.String(), "", fmt.Errorf("Windows IcmpSendEcho: %w", callErr)
	}
	return elapsed, ip.String(), selectedSource(ip.String(), sourceText), nil
}
