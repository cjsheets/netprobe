package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

var base = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func ok(name string, at time.Time, ms float64) model.Observation {
	return model.Observation{Timestamp: at, Target: name, ProbeType: model.ICMP, Success: true, LatencyMS: &ms}
}
func fail(name string, at time.Time, p model.ProbeType) model.Observation {
	return model.Observation{Timestamp: at, Target: name, ProbeType: p, ErrorCategory: "timeout"}
}
func target(name string, tags ...string) model.Target { return model.Target{Name: name, Tags: tags} }

func TestFiveSecondOutageIsOneBoundedIncident(t *testing.T) {
	var obs []model.Observation
	for i := 0; i < 10; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		if i >= 2 && i <= 6 {
			obs = append(obs, fail("public", at, model.ICMP))
		} else {
			obs = append(obs, ok("public", at, 10))
		}
	}
	got := (Detector{Window: 2 * time.Second}).Detect(obs, []model.Target{target("public", "public")}, nil, nil)
	if len(got) != 1 {
		t.Fatalf("incidents=%d", len(got))
	}
	if !got[0].Start.Equal(base.Add(2*time.Second)) || got[0].End == nil || !got[0].End.Equal(base.Add(6*time.Second)) {
		t.Fatalf("bounds=%s..%v", got[0].Start, got[0].End)
	}
	if len(got[0].AffectedTargets) != 1 {
		t.Fatalf("affected=%v", got[0].AffectedTargets)
	}
}

func TestClassifications(t *testing.T) {
	tests := []struct {
		name, want string
		targets    []model.Target
		obs        []model.Observation
	}{
		{"local", "local network", []model.Target{target("router", "local"), target("pub", "public")}, []model.Observation{fail("router", base, model.ICMP), fail("pub", base, model.ICMP)}},
		{"isp", "ISP or upstream", []model.Target{target("router", "local"), target("pub", "public")}, []model.Observation{ok("router", base, 2), fail("pub", base, model.ICMP)}},
		{"vpn", "VPN or work network", []model.Target{target("pub", "public"), target("work", "vpn", "work")}, []model.Observation{ok("pub", base, 10), fail("work", base, model.DNS)}},
		{"service", "remote service", []model.Target{target("pub", "public"), target("rdp", "service")}, []model.Observation{ok("pub", base, 10), fail("rdp", base, model.TCP)}},
		{"ambiguous", "ambiguous", []model.Target{target("mystery")}, []model.Observation{fail("mystery", base, model.TCP)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (Detector{Window: time.Second}).Detect(tt.obs, tt.targets, nil, nil)
			if len(got) != 1 || got[0].Scope != tt.want {
				t.Fatalf("got %#v want %q", got, tt.want)
			}
			if !strings.Contains(got[0].Explanation, "Likely ") {
				t.Fatalf("not conservative: %s", got[0].Explanation)
			}
		})
	}
}

func TestLatencyIncidentAndGapExclusion(t *testing.T) {
	obs := []model.Observation{ok("pub", base, 10), ok("pub", base.Add(time.Second), 10), ok("pub", base.Add(2*time.Second), 10), ok("pub", base.Add(3*time.Second), 500)}
	got := (Detector{Window: time.Second}).Detect(obs, []model.Target{target("pub", "public")}, nil, nil)
	if len(got) != 1 || got[0].LatencyChangeMS != 500 {
		t.Fatalf("latency incident=%#v", got)
	}
	gaps := []model.Gap{{Start: base.Add(3 * time.Second), End: base.Add(4 * time.Second), Kind: "sleep_or_wake"}}
	got = (Detector{Window: time.Second}).Detect(obs, []model.Target{target("pub", "public")}, gaps, nil)
	if len(got) != 0 {
		t.Fatalf("gap became incident: %#v", got)
	}
}

func TestRouteChangeAssociatedWithIncident(t *testing.T) {
	g := []model.Gap{{Start: base, End: base, Kind: "network_change", Details: "work route changed"}}
	got := (Detector{Window: time.Second}).Detect([]model.Observation{fail("work", base, model.TCP)}, []model.Target{target("work", "work")}, g, nil)
	if len(got) != 1 || !strings.Contains(got[0].Explanation, "route changed") {
		t.Fatalf("route evidence=%#v", got)
	}
}
