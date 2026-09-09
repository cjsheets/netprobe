package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chadsheets/netprobe/internal/app"
	"github.com/chadsheets/netprobe/internal/config"
	"github.com/chadsheets/netprobe/internal/incident"
	"github.com/chadsheets/netprobe/internal/model"
	"github.com/chadsheets/netprobe/internal/report"
	"github.com/chadsheets/netprobe/internal/storage"
	"github.com/chadsheets/netprobe/internal/tui"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if e := run(os.Args[1:], os.Stdout, os.Stderr); e != nil {
		fmt.Fprintln(os.Stderr, "netprobe:", e)
		os.Exit(1)
	}
}
func run(args []string, out, errOut io.Writer) error {
	root := flag.NewFlagSet("netprobe", flag.ContinueOnError)
	root.SetOutput(errOut)
	cfgPath := root.String("config", config.DefaultPath(), "configuration file")
	if e := root.Parse(args); e != nil {
		return e
	}
	args = root.Args()
	if len(args) == 0 {
		return usage(errOut)
	}
	cmd, args := args[0], args[1:]
	if cmd == "version" {
		fmt.Fprintln(out, version)
		return nil
	}
	if cmd == "config" && len(args) > 0 && args[0] == "check" {
		c, e := config.Load(*cfgPath)
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "configuration is valid: %d targets, database %s\n", len(c.Targets), c.Database)
		return nil
	}
	c, e := config.Load(*cfgPath)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(c.Database), 0700); e != nil {
		return e
	}
	s, e := storage.Open(c.Database)
	if e != nil {
		return e
	}
	defer s.Close()
	ctx := context.Background()
	switch cmd {
	case "run":
		sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Fprintf(out, "collecting %d configured targets; database %s\n", len(c.Targets), c.Database)
		return (&app.Collector{Config: c, Store: s}).Run(sigCtx)
	case "mark":
		msg := strings.TrimSpace(strings.Join(args, " "))
		if msg == "" {
			msg = "user marker"
		}
		if e = s.AddMarker(ctx, time.Now().UTC(), msg); e == nil {
			fmt.Fprintf(out, "marker recorded: %s\n", msg)
		}
		return e
	case "status":
		obs, inc, gap, marks, e := s.Counts(ctx)
		if e != nil {
			return e
		}
		last, _ := s.LastObservationTime(ctx)
		fmt.Fprintf(out, "database: %s\nobservations: %d\nincidents: %d\ngaps/changes: %d\nmarkers: %d\nlast observation: %s\n", c.Database, obs, inc, gap, marks, formatTime(last))
		return nil
	case "watch":
		return watch(ctx, s, args, out)
	case "incidents":
		return listIncidents(ctx, s, args, out)
	case "report":
		return makeReport(ctx, s, c, args, out, true)
	case "export":
		return makeReport(ctx, s, c, args, out, false)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}
func usage(w io.Writer) error {
	fmt.Fprintln(w, `usage: netprobe [--config PATH] COMMAND

commands: run, watch, status, mark [message], incidents, report, export,
          config check, version`)
	return errors.New("a command is required")
}
func parseRange(fs *flag.FlagSet, args []string) (time.Time, time.Time, error) {
	from := fs.String("from", "", "start time (RFC3339)")
	to := fs.String("to", "", "end time (RFC3339)")
	since := fs.Duration("since", time.Hour, "relative time window")
	if e := fs.Parse(args); e != nil {
		return time.Time{}, time.Time{}, e
	}
	end := time.Now().UTC()
	var e error
	if *to != "" {
		end, e = time.Parse(time.RFC3339, *to)
		if e != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--to: %w", e)
		}
	}
	start := end.Add(-*since)
	if *from != "" {
		start, e = time.Parse(time.RFC3339, *from)
		if e != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("--from: %w", e)
		}
	}
	if !start.Before(end) {
		return start, end, errors.New("--from must be before --to")
	}
	return start, end, nil
}
func watch(ctx context.Context, s *storage.Store, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	once := fs.Bool("once", false, "render once")
	noColor := fs.Bool("no-color", false, "disable color")
	ascii := fs.Bool("ascii", false, "use ASCII graphics")
	width := fs.Int("width", 0, "terminal width (default: detect)")
	if e := fs.Parse(args); e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	selected := 0
	keys := make(chan byte, 8)
	rawTerminal := false
	if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if old, e := term.MakeRaw(int(os.Stdin.Fd())); e == nil {
			rawTerminal = true
			defer term.Restore(int(os.Stdin.Fd()), old)
			go func() {
				buf := make([]byte, 1)
				for {
					if _, e := os.Stdin.Read(buf); e != nil {
						return
					}
					select {
					case keys <- buf[0]:
					case <-ctx.Done():
						return
					}
				}
			}()
		}
	}
	for {
		to := time.Now().UTC()
		from := to.Add(-60 * time.Second)
		obs, e := s.Observations(ctx, from, to)
		if e != nil {
			return e
		}
		ins, _ := s.Incidents(ctx, from, to)
		marks, _ := s.Markers(ctx, from, to)
		actualWidth := *width
		if actualWidth == 0 {
			actualWidth = 80
			if f, ok := out.(*os.File); ok {
				if w, _, e := term.GetSize(int(f.Fd())); e == nil {
					actualWidth = w
				}
			}
		}
		if !*once {
			fmt.Fprint(out, "\x1b[H\x1b[2J")
		}
		rendered := tui.Render(obs, ins, marks, tui.Options{Width: actualWidth, Unicode: !*ascii, Color: !*noColor, Selected: selected})
		if rawTerminal {
			rendered = strings.ReplaceAll(rendered, "\n", "\r\n")
		}
		fmt.Fprint(out, rendered)
		if *once {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case key := <-keys:
			switch key {
			case 'q', 'Q':
				return nil
			case 'j', 'B':
				selected++
			case 'k', 'A':
				selected--
			}
			if selected < 0 {
				selected = 0
			}
		case <-time.After(time.Second):
		}
	}
}
func listIncidents(ctx context.Context, s *storage.Store, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("incidents", flag.ContinueOnError)
	scope := fs.String("scope", "", "filter likely scope")
	from, to, e := parseRange(fs, args)
	if e != nil {
		return e
	}
	ins, e := s.Incidents(ctx, from, to)
	if e != nil {
		return e
	}
	shown := 0
	for _, in := range ins {
		if *scope == "" || strings.EqualFold(*scope, in.Scope) {
			fmt.Fprintf(out, "%d  %s\n    %s\n", in.ID, incident.Summary(in), in.Explanation)
			shown++
		}
	}
	if shown == 0 {
		fmt.Fprintln(out, "no incidents in selected range")
	}
	return nil
}
func makeReport(ctx context.Context, s *storage.Store, c *config.Config, args []string, out io.Writer, isReport bool) error {
	fs := flag.NewFlagSet("output", flag.ContinueOnError)
	format := fs.String("format", map[bool]string{true: "html", false: "csv"}[isReport], "html, csv, or json")
	output := fs.String("output", "-", "output path, or - for stdout")
	incidentID := fs.Int64("incident", 0, "incident ID")
	from, to, e := parseRange(fs, args)
	if e != nil {
		return e
	}
	ins, e := s.Incidents(ctx, from, to)
	if e != nil {
		return e
	}
	if *incidentID > 0 {
		all, e := s.Incidents(ctx, time.Unix(0, 0), time.Now().Add(24*time.Hour))
		if e != nil {
			return e
		}
		found := false
		for _, in := range all {
			if in.ID == *incidentID {
				ins = []model.Incident{in}
				from = in.Start.Add(-30 * time.Second)
				if in.End != nil {
					to = in.End.Add(30 * time.Second)
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("incident %d not found", *incidentID)
		}
	}
	obs, e := s.Observations(ctx, from, to)
	if e != nil {
		return e
	}
	marks, _ := s.Markers(ctx, from, to)
	gaps, _ := s.Gaps(ctx, from, to)
	rollups, _ := s.Rollups(ctx, from, to)
	b := report.Bundle{GeneratedAt: time.Now().UTC(), From: from, To: to, Targets: report.SafeTargets(c.Targets), Observations: obs, Rollups: rollups, Incidents: ins, Markers: marks, Gaps: gaps}
	w := out
	var f *os.File
	if *output != "-" {
		f, e = os.Create(*output)
		if e != nil {
			return e
		}
		defer f.Close()
		w = f
	}
	switch *format {
	case "json":
		e = report.JSON(w, b)
	case "csv":
		if isReport {
			return errors.New("report supports --format html|json; use export for csv")
		}
		e = report.CSV(w, obs)
	case "html":
		if !isReport {
			return errors.New("export supports --format csv|json; use report for html")
		}
		e = report.HTML(w, b)
	default:
		e = fmt.Errorf("unsupported format %q", *format)
	}
	if e == nil && f != nil {
		fmt.Fprintf(out, "wrote %s (%s)\n", *output, *format)
	}
	return e
}
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Local().Format(time.RFC3339)
}
