// Package latency samples round-trip time from another client while a workload runs, because "the
// server was blocked" is a claim about everybody else, and the only way to have it is to be
// somebody else.
package latency

import (
	"sort"
	"sync"
	"time"
)

// Percentile returns the nearest-rank percentile of a set of durations. The samples are the
// caller's; this function does not sort them.
func Percentile(samples []time.Duration, p float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	idx := int(float64(len(samples)) * p)
	if idx >= len(samples) {
		idx = len(samples) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return samples[idx]
}

// Sorted returns the samples in place, so a caller can take several percentiles without re-sorting.
func Sorted(samples []time.Duration) []time.Duration {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples
}

// Sampler runs one probe in a goroutine and records what it saw. It is deliberately one probe:
// a thousand probes measure the client's throughput, and the claim here is about a single caller's
// worst round trip.
type Sampler struct {
	probe   func() error
	mu      sync.Mutex
	samples []time.Duration
	errors  int
	stop    chan struct{}
	done    chan struct{}
}

func NewSampler(probe func() error) *Sampler {
	return &Sampler{probe: probe, stop: make(chan struct{}), done: make(chan struct{})}
}

func (s *Sampler) Start() {
	go func() {
		defer close(s.done)
		for {
			select {
			case <-s.stop:
				return
			default:
			}
			start := time.Now()
			err := s.probe()
			elapsed := time.Since(start)
			s.mu.Lock()
			s.samples = append(s.samples, elapsed)
			if err != nil {
				s.errors++
			}
			s.mu.Unlock()
		}
	}()
}

func (s *Sampler) Stop() {
	close(s.stop)
	<-s.done
}

// Result returns the sorted samples and the number of probes that came back an error. The error
// count is part of the latency story: time spent waiting on a rejected connection is not latency.
func (s *Sampler) Result() ([]time.Duration, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]time.Duration(nil), s.samples...)
	return Sorted(out), s.errors
}
