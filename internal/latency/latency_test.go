package latency_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lasttoss/valkey-scale-lab/internal/latency"
)

func TestPercentileIsNearestRank(t *testing.T) {
	// Ten samples, so the answer is checkable by eye: the 50th percentile of the first five is the
	// fifth, and the 99th is the tenth.
	s := []time.Duration{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, tc := range []struct {
		p    float64
		want time.Duration
	}{{0.5, 6}, {0.9, 10}, {0.99, 10}, {0, 1}} {
		if got := latency.Percentile(s, tc.p); got != tc.want {
			t.Errorf("Percentile(p=%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
	if got := latency.Percentile(nil, 0.99); got != 0 {
		t.Errorf("Percentile of nothing = %v, want 0", got)
	}
}

func TestSamplerCountsItsErrors(t *testing.T) {
	n := 0
	s := latency.NewSampler(func() error {
		n++
		if n%2 == 0 {
			return errors.New("max number of clients reached")
		}
		return nil
	})
	s.Start()
	time.Sleep(5 * time.Millisecond)
	s.Stop()
	samples, errs := s.Result()
	if len(samples) == 0 {
		t.Fatal("no samples: the probe never ran")
	}
	if errs == 0 {
		t.Error("errors = 0, but every second probe failed")
	}
	if len(samples) != n {
		t.Errorf("%d samples for %d probes", len(samples), n)
	}
}
