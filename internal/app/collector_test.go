package app

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chadsheets/netprobe/internal/config"
	"github.com/chadsheets/netprobe/internal/model"
	"github.com/chadsheets/netprobe/internal/storage"
)

type fakeProbe struct {
	mu sync.Mutex
	n  int
}

func (f *fakeProbe) Probe(_ context.Context, t model.Target) model.Observation {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	ms := 5.0
	return model.Observation{Timestamp: time.Now().UTC(), Target: t.Name, ProbeType: t.Type, Success: f.n > 5, LatencyMS: &ms}
}

func TestCollectorPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	s, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	yes := true
	c := &config.Config{Database: path, Concurrency: 2, RawRetentionDuration: time.Hour, RollupRetentionDuration: 24 * time.Hour, IncidentWindowDuration: 20 * time.Millisecond, Targets: []model.Target{{Name: "x", Type: model.TCP, Interval: 10 * time.Millisecond, Timeout: 5 * time.Millisecond, Enabled: &yes}}}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	if e = (&Collector{Config: c, Store: s, Prober: &fakeProbe{}}).Run(ctx); e != nil {
		t.Fatal(e)
	}
	obs, _, _, _, e := s.Counts(context.Background())
	if e != nil || obs < 5 {
		t.Fatalf("observations=%d err=%v", obs, e)
	}
	s.Close()
	s, e = storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	obs2, _, _, _, _ := s.Counts(context.Background())
	if obs2 != obs {
		t.Fatalf("restart lost data: %d != %d", obs2, obs)
	}
}

func TestTimingGapDetection(t *testing.T) {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := detectTimingGaps(a, a.Add(10*time.Second), 10*time.Second, 3*time.Second)
	if len(g) != 1 || g[0].Kind != "sleep_or_wake" {
		t.Fatalf("sleep gaps=%#v", g)
	}
	g = detectTimingGaps(a, a.Add(10*time.Second), time.Second, 3*time.Second)
	if len(g) != 2 || g[0].Kind != "sleep_or_wake" || g[1].Kind != "clock_change" {
		t.Fatalf("clock gaps=%#v", g)
	}
}
