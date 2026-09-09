package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/chadsheets/netprobe/internal/model"
)

func TestRenderNarrowAndWide(t *testing.T) {
	ms := 42.0
	obs := []model.Observation{{Timestamp: time.Now(), Target: "a-very-long-target-name", Success: true, LatencyMS: &ms}, {Timestamp: time.Now(), Target: "a-very-long-target-name"}}
	for _, w := range []int{80, 160} {
		got := Render(obs, nil, nil, Options{Width: w, Unicode: w > 80})
		if !strings.Contains(got, "X") || !strings.Contains(got, "LOSS") {
			t.Fatalf("width %d output %q", w, got)
		}
		for _, line := range strings.Split(got, "\n") {
			if len([]rune(line)) > w {
				t.Fatalf("width %d line too long (%d): %q", w, len([]rune(line)), line)
			}
		}
	}
}
