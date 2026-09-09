package app

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chadsheets/netprobe/internal/config"
	"github.com/chadsheets/netprobe/internal/incident"
	"github.com/chadsheets/netprobe/internal/model"
	"github.com/chadsheets/netprobe/internal/probe"
	"github.com/chadsheets/netprobe/internal/storage"
)

type Collector struct {
	Config *config.Config
	Store  *storage.Store
	Prober probe.Prober
	Now    func() time.Time
}

func (c *Collector) Run(ctx context.Context) error {
	if c.Prober == nil {
		c.Prober = probe.Native{}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	enabled := []model.Target{}
	minInterval := time.Duration(0)
	for _, t := range c.Config.Targets {
		if t.IsEnabled() {
			enabled = append(enabled, t)
			if minInterval == 0 || t.Interval < minInterval {
				minInterval = t.Interval
			}
		}
	}
	if len(enabled) == 0 {
		return fmt.Errorf("no enabled targets")
	}
	last, _ := c.Store.LastObservationTime(ctx)
	now := c.Now()
	if !last.IsZero() && now.Sub(last) > 2*minInterval+time.Second {
		_ = c.Store.AddGap(ctx, model.Gap{Start: last, End: now, Kind: "collector_downtime", Details: "collector was not running"})
	}
	results := make(chan model.Observation, len(enabled)*2)
	sem := make(chan struct{}, c.Config.Concurrency)
	var schedulers sync.WaitGroup
	for _, target := range enabled {
		schedulers.Add(1)
		go func(t model.Target) { defer schedulers.Done(); runSchedule(ctx, t, c.Prober, sem, results, c.Now) }(target)
	}
	done := make(chan struct{})
	go func() { schedulers.Wait(); close(results); close(done) }()
	lastWall, lastMono := now, time.Now()
	routes := map[string]string{}
	maintenance := time.NewTicker(time.Minute)
	defer maintenance.Stop()
	incidentTick := time.NewTicker(5 * time.Second)
	defer incidentTick.Stop()
	_ = c.refreshIncidents(context.Background())
	for {
		select {
		case o, ok := <-results:
			if !ok {
				return nil
			}
			wall, mono := c.Now(), time.Now()
			monoDelta := mono.Sub(lastMono)
			for _, gap := range detectTimingGaps(lastWall, wall, monoDelta, 2*minInterval+time.Second) {
				_ = c.Store.AddGap(ctx, gap)
			}
			lastWall, lastMono = wall, mono
			if prev, ok := routes[o.Target]; ok && o.Route != "" && prev != o.Route {
				_ = c.Store.AddGap(ctx, model.Gap{Start: o.Timestamp, End: o.Timestamp, Kind: "network_change", Details: fmt.Sprintf("%s route changed from %s to %s", o.Target, prev, o.Route)})
			}
			if o.Route != "" {
				routes[o.Target] = o.Route
			}
			if e := c.Store.InsertObservation(ctx, o); e != nil {
				return e
			}
		case <-maintenance.C:
			_ = c.Store.RollupAndRetain(context.Background(), c.Config.RawRetentionDuration, c.Config.RollupRetentionDuration)
		case <-incidentTick.C:
			_ = c.refreshIncidents(context.Background())
		case <-ctx.Done():
			<-done
			_ = c.refreshIncidents(context.Background())
			return nil
		}
	}
}

func runSchedule(ctx context.Context, t model.Target, p probe.Prober, sem chan struct{}, out chan<- model.Observation, now func() time.Time) {
	base := now()
	for n := int64(0); ; n++ {
		due := base.Add(time.Duration(n) * t.Interval)
		timer := time.NewTimer(time.Until(due))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		o := p.Probe(ctx, t)
		<-sem
		select {
		case out <- o:
		case <-ctx.Done():
			return
		}
	}
}
func (c *Collector) refreshIncidents(ctx context.Context) error {
	to := time.Now().UTC()
	window := 10 * time.Minute
	if c.Config.RawRetentionDuration < window {
		window = c.Config.RawRetentionDuration
	}
	from := to.Add(-window)
	existing, _ := c.Store.Incidents(ctx, to.Add(-c.Config.RawRetentionDuration), to)
	for _, in := range existing {
		if in.End == nil && in.Start.Before(from) {
			from = in.Start.Add(-c.Config.IncidentWindowDuration)
		}
	}
	obs, e := c.Store.Observations(ctx, from, to)
	if e != nil {
		return e
	}
	gaps, _ := c.Store.Gaps(ctx, from, to)
	markers, _ := c.Store.Markers(ctx, from, to)
	ins := incident.Detector{Window: c.Config.IncidentWindowDuration}.Detect(obs, c.Config.Targets, gaps, markers)
	return c.Store.ReplaceIncidentsFrom(ctx, from, ins)
}
func mathAbsDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func detectTimingGaps(lastWall, wall time.Time, monotonicDelta, gapThreshold time.Duration) []model.Gap {
	var gaps []model.Gap
	wallDelta := wall.Sub(lastWall)
	if wallDelta > gapThreshold || monotonicDelta > gapThreshold {
		gaps = append(gaps, model.Gap{Start: lastWall, End: wall, Kind: "sleep_or_wake", Details: "sampling paused while host was suspended or heavily delayed"})
	}
	if mathAbsDuration(wallDelta-monotonicDelta) > 2*time.Second {
		gaps = append(gaps, model.Gap{Start: lastWall, End: wall, Kind: "clock_change", Details: fmt.Sprintf("wall clock changed by %s relative to monotonic time", wallDelta-monotonicDelta)})
	}
	return gaps
}
