package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

func bundle() Bundle {
	ms := 1.25
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return Bundle{From: at, To: at.Add(time.Minute), Targets: []model.Target{{Name: "<unsafe>", Type: model.TCP}}, Observations: []model.Observation{{Timestamp: at, Target: "<unsafe>", ProbeType: model.TCP, Success: true, LatencyMS: &ms}}, Gaps: []model.Gap{{Start: at, End: at, Kind: "clock_change"}}}
}
func TestFormats(t *testing.T) {
	var b bytes.Buffer
	if e := JSON(&b, bundle()); e != nil {
		t.Fatal(e)
	}
	var decoded map[string]any
	if e := json.Unmarshal(b.Bytes(), &decoded); e != nil || decoded["schema"] != SchemaVersion {
		t.Fatalf("json %v %#v", e, decoded)
	}
	b.Reset()
	if e := CSV(&b, bundle().Observations); e != nil {
		t.Fatal(e)
	}
	rows, e := csv.NewReader(&b).ReadAll()
	if e != nil || len(rows) != 2 || len(rows[0]) != 11 {
		t.Fatalf("csv rows=%v err=%v", rows, e)
	}
	b.Reset()
	if e := HTML(&b, bundle()); e != nil {
		t.Fatal(e)
	}
	s := b.String()
	if !strings.Contains(s, "<!doctype html>") || strings.Contains(s, "src=\"http") {
		t.Fatalf("not standalone HTML")
	}
	if strings.Contains(s, "<unsafe></td>") {
		t.Fatalf("unsafe value rendered as markup")
	}
}

func TestSafeTargetsRedactsURLSecrets(t *testing.T) {
	got := SafeTargets([]model.Target{{Name: "x", Type: model.HTTP, URL: "https://user:pass@example.test/health?token=secret#x", Metadata: map[string]string{"secret": "value"}}})
	if got[0].URL != "https://example.test/health" || got[0].Metadata != nil {
		t.Fatalf("unsafe target=%#v", got[0])
	}
}
