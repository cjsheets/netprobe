package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Database                string         `yaml:"database"`
	Concurrency             int            `yaml:"concurrency"`
	RawRetention            string         `yaml:"raw_retention"`
	RollupRetention         string         `yaml:"rollup_retention"`
	IncidentWindow          string         `yaml:"incident_window"`
	Targets                 []model.Target `yaml:"targets"`
	RawRetentionDuration    time.Duration  `yaml:"-"`
	RollupRetentionDuration time.Duration  `yaml:"-"`
	IncidentWindowDuration  time.Duration  `yaml:"-"`
}

func DefaultPath() string {
	if p := os.Getenv("NETPROBE_CONFIG"); p != "" {
		return p
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "netprobe", "config.yaml")
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read configuration %q: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse configuration %q: %w", path, err)
	}
	if c.Database == "" {
		c.Database = "netprobe.db"
	}
	if !filepath.IsAbs(c.Database) {
		c.Database = filepath.Join(filepath.Dir(path), c.Database)
	}
	if c.Concurrency == 0 {
		c.Concurrency = 32
	}
	if c.RawRetention == "" {
		c.RawRetention = "336h"
	}
	if c.RollupRetention == "" {
		c.RollupRetention = "8760h"
	}
	if c.IncidentWindow == "" {
		c.IncidentWindow = "5s"
	}
	var problems []string
	parse := func(label, value string, out *time.Duration) {
		d, e := time.ParseDuration(value)
		if e != nil || d <= 0 {
			problems = append(problems, fmt.Sprintf("%s must be a positive duration (for example 14d is written 336h); got %q", label, value))
			return
		}
		*out = d
	}
	parse("raw_retention", c.RawRetention, &c.RawRetentionDuration)
	parse("rollup_retention", c.RollupRetention, &c.RollupRetentionDuration)
	parse("incident_window", c.IncidentWindow, &c.IncidentWindowDuration)
	if c.Concurrency < 1 || c.Concurrency > 1024 {
		problems = append(problems, "concurrency must be between 1 and 1024")
	}
	if len(c.Targets) == 0 {
		problems = append(problems, "targets must contain at least one target")
	}
	seen := map[string]bool{}
	for i := range c.Targets {
		validateTarget(&c.Targets[i], i, seen, &problems)
	}
	if len(problems) > 0 {
		return nil, errors.New("configuration is invalid:\n - " + strings.Join(problems, "\n - "))
	}
	return &c, nil
}

func validateTarget(t *model.Target, i int, seen map[string]bool, problems *[]string) {
	p := fmt.Sprintf("targets[%d]", i)
	if strings.TrimSpace(t.Name) == "" {
		*problems = append(*problems, p+".name is required")
	} else if seen[t.Name] {
		*problems = append(*problems, p+".name must be unique; duplicate "+fmt.Sprintf("%q", t.Name))
	} else {
		seen[t.Name] = true
	}
	parse := func(field, raw string, fallback time.Duration) time.Duration {
		if raw == "" {
			return fallback
		}
		d, e := time.ParseDuration(raw)
		if e != nil || d <= 0 {
			*problems = append(*problems, fmt.Sprintf("%s.%s must be a positive duration; got %q", p, field, raw))
			return fallback
		}
		return d
	}
	t.Interval = parse("interval", t.IntervalRaw, time.Second)
	t.Timeout = parse("timeout", t.TimeoutRaw, 900*time.Millisecond)
	if t.Timeout > t.Interval {
		*problems = append(*problems, p+".timeout must not exceed interval (increase interval or lower timeout)")
	}
	switch t.Type {
	case model.ICMP:
		if t.Host == "" {
			*problems = append(*problems, p+".host is required for icmp")
		}
	case model.TCP:
		if t.Host == "" {
			*problems = append(*problems, p+".host is required for tcp")
		}
		if t.Port < 1 || t.Port > 65535 {
			*problems = append(*problems, p+".port must be between 1 and 65535 for tcp")
		}
	case model.DNS:
		if t.Resolver == "" {
			*problems = append(*problems, p+".resolver is required for dns (host or host:port)")
		}
		if t.Query == "" {
			*problems = append(*problems, p+".query is required for dns")
		}
	case model.HTTP:
		u, e := url.Parse(t.URL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			*problems = append(*problems, p+".url must be a valid http:// or https:// URL")
		} else if u.User != nil {
			*problems = append(*problems, p+".url must not contain embedded credentials; use an unauthenticated health endpoint")
		}
	default:
		*problems = append(*problems, p+".type must be one of: icmp, tcp, dns, http")
	}
	if t.Source != "" && net.ParseIP(t.Source) == nil {
		*problems = append(*problems, p+".source must be an IP address")
	}
	if t.Source != "" && t.Interface != "" {
		*problems = append(*problems, p+" must set only one of source or interface")
	}
}
