package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

func TestNativeSuccessAndTimeout(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			c.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	o := (Native{}).Probe(context.Background(), model.Target{Name: "tcp", Type: model.TCP, Host: "127.0.0.1", Port: addr.Port, Timeout: time.Second})
	if !o.Success || o.LatencyMS == nil {
		t.Fatalf("tcp result=%#v", o)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	o = (Native{}).Probe(context.Background(), model.Target{Name: "http", Type: model.HTTP, URL: srv.URL, Timeout: 20 * time.Millisecond})
	if o.Success || o.ErrorCategory != "timeout" {
		t.Fatalf("timeout result=%#v", o)
	}
}
