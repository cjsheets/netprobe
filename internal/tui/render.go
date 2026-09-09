package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chadsheets/netprobe/internal/incident"
	"github.com/chadsheets/netprobe/internal/model"
)

type Options struct {
	Width    int
	Unicode  bool
	Color    bool
	Selected int
}

func Render(obs []model.Observation, ins []model.Incident, marks []model.Marker, opt Options) string {
	if opt.Width < 40 {
		opt.Width = 80
	}
	by := map[string][]model.Observation{}
	for _, o := range obs {
		by[o.Target] = append(by[o.Target], o)
	}
	names := make([]string, 0, len(by))
	for n := range by {
		names = append(names, n)
	}
	sort.Strings(names)
	if opt.Selected < 0 {
		opt.Selected = 0
	}
	if len(names) > 0 && opt.Selected >= len(names) {
		opt.Selected = len(names) - 1
	}
	var timelineStart, timelineEnd time.Time
	for _, o := range obs {
		if timelineStart.IsZero() || o.Timestamp.Before(timelineStart) {
			timelineStart = o.Timestamp
		}
		if timelineEnd.IsZero() || o.Timestamp.After(timelineEnd) {
			timelineEnd = o.Timestamp
		}
	}
	timeline := opt.Width - 38
	if timeline < 12 {
		timeline = 12
	}
	var b strings.Builder
	fmt.Fprintf(&b, "netprobe  %s  %d target(s)\n", time.Now().Format("15:04:05"), len(names))
	fmt.Fprintf(&b, "%-18s %-8s %-7s %s\n", "TARGET", "LATENCY", "LOSS", "ALIGNED RECENT TIMELINE")
	chars := " .:-=+*#%@"
	if opt.Unicode {
		chars = " ▂▃▄▅▆▇█"
	}
	position := func(at time.Time) int {
		if timelineEnd.Equal(timelineStart) {
			return timeline - 1
		}
		p := int(float64(at.Sub(timelineStart)) / float64(timelineEnd.Sub(timelineStart)) * float64(timeline-1))
		if p < 0 {
			return 0
		}
		if p >= timeline {
			return timeline - 1
		}
		return p
	}
	for rowIndex, n := range names {
		v := by[n]
		loss := 0
		var latest string = "—"
		line := make([]rune, timeline)
		for i := range line {
			line[i] = ' '
		}
		for _, o := range v {
			pos := position(o.Timestamp)
			if !o.Success {
				loss++
				line[pos] = 'X'
			} else if o.LatencyMS != nil {
				latest = fmt.Sprintf("%.1fms", *o.LatencyMS)
				idx := int(*o.LatencyMS/25) + 1
				r := []rune(chars)
				if idx >= len(r) {
					idx = len(r) - 1
				}
				line[pos] = r[idx]
			} else {
				line[pos] = '?'
			}
		}
		pct := 0.0
		if len(v) > 0 {
			pct = 100 * float64(loss) / float64(len(v))
		}
		prefix := "  "
		if rowIndex == opt.Selected {
			prefix = "> "
		}
		fmt.Fprintf(&b, "%-18s %-8s %5.1f%%  %s\n", prefix+trim(n, 16), latest, pct, string(line))
	}
	if len(marks) > 0 && !timelineStart.IsZero() {
		line := make([]rune, timeline)
		for i := range line {
			line[i] = ' '
		}
		for _, m := range marks {
			if !m.Timestamp.Before(timelineStart) && !m.Timestamp.After(timelineEnd) {
				line[position(m.Timestamp)] = '|'
			}
		}
		fmt.Fprintf(&b, "%-18s %-8s %-7s %s\n", "  markers", "—", "—", string(line))
	}
	if len(ins) > 0 && !timelineStart.IsZero() {
		line := make([]rune, timeline)
		for i := range line {
			line[i] = ' '
		}
		for _, in := range ins {
			end := timelineEnd
			if in.End != nil {
				end = *in.End
			}
			if end.Before(timelineStart) || in.Start.After(timelineEnd) {
				continue
			}
			a, z := position(in.Start), position(end)
			if z < a {
				a, z = z, a
			}
			for i := a; i <= z; i++ {
				line[i] = '='
			}
			line[a] = '!'
		}
		fmt.Fprintf(&b, "%-18s %-8s %-7s %s\n", "  incidents", "—", "—", string(line))
	}
	if len(names) > 0 {
		v := by[names[opt.Selected]]
		if len(v) > 0 {
			last := v[len(v)-1]
			via := last.Route
			if via == "" {
				via = last.Source + " -> " + last.Destination
			}
			detail := fmt.Sprintf("> %s via %s [%s]", names[opt.Selected], via, last.Interface)
			fmt.Fprintf(&b, "\n%s\n", trim(detail, opt.Width))
		}
	}
	if len(ins) > 0 {
		in := ins[len(ins)-1]
		fmt.Fprintf(&b, "\n! %s\n  %s\n", incident.Summary(in), in.Explanation)
	}
	if len(marks) > 0 {
		m := marks[len(marks)-1]
		fmt.Fprintf(&b, "\n| marker %s: %s\n", m.Timestamp.Local().Format("15:04:05"), m.Message)
	}
	fmt.Fprint(&b, "\nLegend: X loss/timeout, ? no latency; ↑/↓ or j/k navigate, q exits\n")
	return b.String()
}
func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
