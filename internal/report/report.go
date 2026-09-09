package report

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/url"
	"strconv"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

const SchemaVersion = "netprobe.export.v1"

type Bundle struct {
	Schema       string              `json:"schema"`
	GeneratedAt  time.Time           `json:"generated_at"`
	From         time.Time           `json:"from"`
	To           time.Time           `json:"to"`
	Targets      []model.Target      `json:"targets,omitempty"`
	Observations []model.Observation `json:"observations"`
	Rollups      []model.Rollup      `json:"rollups,omitempty"`
	Incidents    []model.Incident    `json:"incidents,omitempty"`
	Markers      []model.Marker      `json:"markers,omitempty"`
	Gaps         []model.Gap         `json:"gaps,omitempty"`
}

func JSON(w io.Writer, b Bundle) error {
	b.Schema = SchemaVersion
	if b.GeneratedAt.IsZero() {
		b.GeneratedAt = time.Now().UTC()
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(b)
}

func CSV(w io.Writer, obs []model.Observation) error {
	c := csv.NewWriter(w)
	defer c.Flush()
	_ = c.Write([]string{"schema", "timestamp_utc", "target", "probe_type", "success", "latency_ms", "error_category", "destination_address", "source_address", "interface", "route"})
	for _, o := range obs {
		lat := ""
		if o.LatencyMS != nil {
			lat = strconv.FormatFloat(*o.LatencyMS, 'f', 3, 64)
		}
		_ = c.Write([]string{SchemaVersion, o.Timestamp.UTC().Format(time.RFC3339Nano), o.Target, string(o.ProbeType), strconv.FormatBool(o.Success), lat, o.ErrorCategory, o.Destination, o.Source, o.Interface, o.Route})
	}
	return c.Error()
}

//go:embed report.html
var htmlSource string
var htmlTemplate = template.Must(template.New("report").Parse(htmlSource))

func HTML(w io.Writer, b Bundle) error {
	b.Schema = SchemaVersion
	if b.GeneratedAt.IsZero() {
		b.GeneratedAt = time.Now().UTC()
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	data := struct {
		From, To string
		Data     template.JS
	}{b.From.Format(time.RFC3339), b.To.Format(time.RFC3339), template.JS(raw)}
	if err = htmlTemplate.Execute(w, data); err != nil {
		return fmt.Errorf("render HTML: %w", err)
	}
	return nil
}

func SafeTargets(targets []model.Target) []model.Target {
	out := make([]model.Target, 0, len(targets))
	for _, t := range targets {
		t.Metadata = nil
		if t.URL != "" {
			if u, e := url.Parse(t.URL); e == nil {
				u.User = nil
				u.RawQuery = ""
				u.Fragment = ""
				t.URL = u.String()
			}
		}
		out = append(out, t)
	}
	return out
}
