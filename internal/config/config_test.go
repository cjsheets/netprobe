package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationActionable(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	if e := os.WriteFile(p, []byte("targets:\n - name: x\n   type: tcp\n   interval: 1s\n   timeout: 2s\n"), 0600); e != nil {
		t.Fatal(e)
	}
	_, e := Load(p)
	if e == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"targets[0].host", "targets[0].port", "timeout must not exceed"} {
		if !strings.Contains(e.Error(), want) {
			t.Errorf("error %q missing %q", e, want)
		}
	}
}

func TestExampleLoads(t *testing.T) {
	c, e := Load(filepath.Join("..", "..", "netprobe.example.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	if len(c.Targets) < 5 {
		t.Fatalf("targets=%d", len(c.Targets))
	}
}
