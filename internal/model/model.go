package model

import "time"

type ProbeType string

const (
	ICMP ProbeType = "icmp"
	TCP  ProbeType = "tcp"
	DNS  ProbeType = "dns"
	HTTP ProbeType = "http"
)

type Target struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Host        string            `yaml:"host,omitempty" json:"host,omitempty"`
	Type        ProbeType         `yaml:"type" json:"probe_type"`
	Port        int               `yaml:"port,omitempty" json:"port,omitempty"`
	Resolver    string            `yaml:"resolver,omitempty" json:"resolver,omitempty"`
	Query       string            `yaml:"query,omitempty" json:"query,omitempty"`
	URL         string            `yaml:"url,omitempty" json:"url,omitempty"`
	Interval    time.Duration     `yaml:"-" json:"interval"`
	Timeout     time.Duration     `yaml:"-" json:"timeout"`
	IntervalRaw string            `yaml:"interval" json:"-"`
	TimeoutRaw  string            `yaml:"timeout" json:"-"`
	Source      string            `yaml:"source,omitempty" json:"source,omitempty"`
	Interface   string            `yaml:"interface,omitempty" json:"interface,omitempty"`
	Enabled     *bool             `yaml:"enabled,omitempty" json:"enabled"`
	Tags        []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Metadata    map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

func (t Target) IsEnabled() bool { return t.Enabled == nil || *t.Enabled }

type Observation struct {
	ID            int64     `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Target        string    `json:"target"`
	ProbeType     ProbeType `json:"probe_type"`
	Success       bool      `json:"success"`
	LatencyMS     *float64  `json:"latency_ms,omitempty"`
	ErrorCategory string    `json:"error_category,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
	Destination   string    `json:"destination_address,omitempty"`
	Source        string    `json:"source_address,omitempty"`
	Interface     string    `json:"interface,omitempty"`
	Route         string    `json:"route,omitempty"`
}

type Gap struct {
	ID      int64     `json:"id"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Kind    string    `json:"kind"`
	Details string    `json:"details,omitempty"`
}

type Marker struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

type Rollup struct {
	Bucket          time.Time `json:"bucket"`
	Target          string    `json:"target"`
	ProbeType       ProbeType `json:"probe_type"`
	Count           int       `json:"count"`
	SuccessfulCount int       `json:"successful_count"`
	PacketLoss      float64   `json:"packet_loss"`
	MinMS           *float64  `json:"min_ms,omitempty"`
	MedianMS        *float64  `json:"median_ms,omitempty"`
	MaxMS           *float64  `json:"max_ms,omitempty"`
	MeanMS          *float64  `json:"mean_ms,omitempty"`
	JitterMS        *float64  `json:"jitter_ms,omitempty"`
	P90MS           *float64  `json:"p90_ms,omitempty"`
	P95MS           *float64  `json:"p95_ms,omitempty"`
	P99MS           *float64  `json:"p99_ms,omitempty"`
}

type Incident struct {
	ID              int64       `json:"id"`
	Start           time.Time   `json:"start"`
	End             *time.Time  `json:"end,omitempty"`
	Scope           string      `json:"likely_scope"`
	Explanation     string      `json:"explanation"`
	AffectedTargets []string    `json:"affected_targets"`
	ProbeTypes      []ProbeType `json:"probe_types"`
	PacketLoss      float64     `json:"packet_loss"`
	LatencyChangeMS float64     `json:"latency_change_ms,omitempty"`
	Markers         []Marker    `json:"nearby_markers,omitempty"`
}
