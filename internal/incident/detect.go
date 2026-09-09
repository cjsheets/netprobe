package incident

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

type Detector struct{ Window time.Duration }
type event struct {
	o     model.Observation
	spike bool
}

func (d Detector) Detect(obs []model.Observation, targets []model.Target, gaps []model.Gap, markers []model.Marker) []model.Incident {
	if d.Window <= 0 {
		d.Window = 5 * time.Second
	}
	tags := map[string]map[string]bool{}
	for _, t := range targets {
		tags[t.Name] = map[string]bool{}
		for _, x := range t.Tags {
			tags[t.Name][x] = true
		}
	}
	baseline := map[string]float64{}
	count := map[string]int{}
	for _, o := range obs {
		if o.Success && o.LatencyMS != nil {
			baseline[o.Target] += *o.LatencyMS
			count[o.Target]++
		}
	}
	for k := range baseline {
		baseline[k] /= float64(count[k])
	}
	var events []event
	for _, o := range obs {
		if inGap(o.Timestamp, gaps) {
			continue
		}
		spike := false
		if o.Success && o.LatencyMS != nil && count[o.Target] >= 3 {
			threshold := math.Max(100, baseline[o.Target]*3)
			spike = *o.LatencyMS > threshold
		}
		if !o.Success || spike {
			events = append(events, event{o, spike})
		}
	}
	if len(events) == 0 {
		return nil
	}
	sort.Slice(events, func(i, j int) bool { return events[i].o.Timestamp.Before(events[j].o.Timestamp) })
	var groups [][]event
	for _, e := range events {
		if len(groups) == 0 || e.o.Timestamp.Sub(groups[len(groups)-1][len(groups[len(groups)-1])-1].o.Timestamp) > d.Window {
			groups = append(groups, []event{e})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], e)
		}
	}
	var out []model.Incident
	for _, g := range groups {
		in := build(g, tags, obs, gaps, markers, d.Window)
		out = append(out, in)
	}
	return out
}

func build(events []event, tags map[string]map[string]bool, all []model.Observation, gaps []model.Gap, markers []model.Marker, window time.Duration) model.Incident {
	start := events[0].o.Timestamp
	end := events[len(events)-1].o.Timestamp
	affected := map[string]bool{}
	probes := map[model.ProbeType]bool{}
	fail := 0
	spikes := 0.0
	for _, e := range events {
		affected[e.o.Target] = true
		probes[e.o.ProbeType] = true
		if !e.o.Success {
			fail++
		}
		if e.spike && e.o.LatencyMS != nil {
			spikes = math.Max(spikes, *e.o.LatencyMS)
		}
	}
	var names []string
	for n := range affected {
		names = append(names, n)
	}
	sort.Strings(names)
	var pts []model.ProbeType
	for p := range probes {
		pts = append(pts, p)
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i] < pts[j] })
	healthy := map[string]bool{}
	for _, o := range all {
		if !o.Timestamp.Before(start.Add(-window)) && !o.Timestamp.After(end.Add(window)) && o.Success {
			healthy[o.Target] = true
		}
	}
	hasAffected := func(tag string) bool {
		for n := range affected {
			if tags[n][tag] {
				return true
			}
		}
		return false
	}
	hasHealthy := func(tag string) bool {
		for n := range healthy {
			if tags[n][tag] && !affected[n] {
				return true
			}
		}
		return false
	}
	scope, evidence := "ambiguous", "the available targets do not isolate a single failure boundary"
	switch {
	case hasAffected("local") && (hasAffected("public") || hasAffected("vpn") || hasAffected("work") || hasAffected("service")):
		scope = "local network"
		evidence = "a local gateway and downstream targets failed in the same interval"
	case hasHealthy("local") && hasAffected("public"):
		scope = "ISP or upstream"
		evidence = "the local gateway remained reachable while public connectivity failed"
	case hasHealthy("public") && (hasAffected("vpn") || hasAffected("work")):
		scope = "VPN or work network"
		evidence = "public connectivity remained healthy while VPN or work targets failed"
	case hasHealthy("public") && hasAffected("service") && !hasAffected("vpn") && !hasAffected("work"):
		scope = "remote service"
		evidence = "general network connectivity remained healthy while the service probe failed"
	}
	if fail == 0 {
		evidence = fmt.Sprintf("latency increased to %.1f ms; %s", spikes, evidence)
	}
	for _, g := range gaps {
		if g.Kind == "network_change" && !g.Start.Before(start.Add(-window)) && !g.Start.After(end.Add(window)) {
			evidence += "; a nearby interface or route change was recorded: " + g.Details
		}
	}
	near := []model.Marker{}
	for _, m := range markers {
		if !m.Timestamp.Before(start.Add(-30*time.Second)) && !m.Timestamp.After(end.Add(30*time.Second)) {
			near = append(near, m)
		}
	}
	total := 0
	fail = 0
	recovered := false
	latest := end
	for _, o := range all {
		if o.Timestamp.After(latest) {
			latest = o.Timestamp
		}
		if affected[o.Target] && !o.Timestamp.Before(start) && !o.Timestamp.After(end) {
			total++
			if !o.Success {
				fail++
			}
		}
		if affected[o.Target] && o.Timestamp.After(end) && o.Success {
			recovered = true
		}
	}
	var endPointer *time.Time
	if recovered || latest.Sub(end) > window {
		endCopy := end
		endPointer = &endCopy
	}
	loss := 0.0
	if total > 0 {
		loss = float64(fail) / float64(total)
	}
	return model.Incident{Start: start, End: endPointer, Scope: scope, Explanation: "Likely " + scope + ": " + evidence + ". This is a conservative inference, not a definitive diagnosis.", AffectedTargets: names, ProbeTypes: pts, PacketLoss: loss, LatencyChangeMS: spikes, Markers: near}
}
func inGap(t time.Time, gaps []model.Gap) bool {
	for _, g := range gaps {
		if g.Kind != "network_change" && !t.Before(g.Start) && !t.After(g.End) {
			return true
		}
	}
	return false
}

func Summary(in model.Incident) string {
	end := time.Now()
	if in.End != nil {
		end = *in.End
	}
	return fmt.Sprintf("%s  %s  %s  %s", in.Start.Local().Format("2006-01-02 15:04:05"), end.Sub(in.Start).Round(time.Second), in.Scope, strings.Join(in.AffectedTargets, ", "))
}
