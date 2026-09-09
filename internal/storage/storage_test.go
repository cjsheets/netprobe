package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

func TestMigrationRestartConcurrentReadAndRetention(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	old := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Minute).Add(5 * time.Second)
	ms := 12.0
	for i := 0; i < 3; i++ {
		if e = s.InsertObservation(ctx, model.Observation{Timestamp: old.Add(time.Duration(i) * time.Second), Target: "a", ProbeType: model.TCP, Success: i < 2, LatencyMS: &ms}); e != nil {
			t.Fatal(e)
		}
	}
	reader, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	if got, e := reader.Observations(ctx, old.Add(-time.Second), old.Add(time.Minute)); e != nil || len(got) != 3 {
		t.Fatalf("concurrent read: %d %v", len(got), e)
	}
	if e = s.RollupAndRetain(ctx, time.Hour, 24*time.Hour); e != nil {
		t.Fatal(e)
	}
	end := old.Add(time.Second)
	wantIncident := model.Incident{Start: old, End: &end, Scope: "ambiguous", Explanation: "Likely ambiguous", AffectedTargets: []string{"a"}, ProbeTypes: []model.ProbeType{model.TCP}, Markers: []model.Marker{{Timestamp: old, Message: "call froze"}}}
	if e = s.SaveIncidents(ctx, []model.Incident{wantIncident}); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	var versions, rollups int
	if e = s.DB.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&versions); e != nil || versions < 2 {
		t.Fatalf("migrations=%d %v", versions, e)
	}
	if e = s.DB.QueryRow("SELECT count(*) FROM rollups_minute").Scan(&rollups); e != nil || rollups < 1 {
		t.Fatalf("rollups=%d %v", rollups, e)
	}
	rr, e := s.Rollups(ctx, old.Add(-time.Minute), old.Add(time.Minute))
	if e != nil || len(rr) != 1 || rr[0].Count != 3 || rr[0].SuccessfulCount != 2 || rr[0].MedianMS == nil {
		t.Fatalf("rollup=%#v err=%v", rr, e)
	}
	stored, e := s.Incidents(ctx, old.Add(-time.Second), end.Add(time.Second))
	if e != nil || len(stored) != 1 || len(stored[0].Markers) != 1 {
		t.Fatalf("stored incident=%#v err=%v", stored, e)
	}
	obs, _, _, _, e := s.Counts(ctx)
	if e != nil || obs != 0 {
		t.Fatalf("retained obs=%d %v", obs, e)
	}
}
