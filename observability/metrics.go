package observability

import "time"

// Stats holds the aggregate counters across every signal a Collector has
// seen. Unlike the ring buffer, these are lifetime totals: they are not
// affected by eviction.
type Stats struct {
	Signals   uint64 `json:"signals"`
	Failed    uint64 `json:"failed"`
	Cancelled uint64 `json:"cancelled"`
	Async     uint64 `json:"async"`
	Children  uint64 `json:"children"`
	Executed  uint64 `json:"executed"`
}

// Metric holds the statistics accumulated for one signal name.
//
// Durations are kept both as time.Duration and as milliseconds: the
// former is what Go callers expect, the latter is what the dashboard
// renders without having to divide.
type Metric struct {
	Name   string `json:"name"`
	Source Source `json:"source"`

	Count     uint64 `json:"count"`
	Failed    uint64 `json:"failed"`
	Cancelled uint64 `json:"cancelled"`
	Async     uint64 `json:"async"`

	// Children is the total number of child units considered across all
	// occurrences; Executed is how many actually ran.
	Children uint64 `json:"children"`
	Executed uint64 `json:"executed"`

	Total time.Duration `json:"total"`
	Min   time.Duration `json:"min"`
	Max   time.Duration `json:"max"`
	Avg   time.Duration `json:"avg"`

	TotalMS float64 `json:"total_ms"`
	MinMS   float64 `json:"min_ms"`
	MaxMS   float64 `json:"max_ms"`
	AvgMS   float64 `json:"avg_ms"`

	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// FailureRate returns the share of occurrences that failed, in [0,1].
func (m Metric) FailureRate() float64 {
	if m.Count == 0 {
		return 0
	}
	return float64(m.Failed) / float64(m.Count)
}

// metric is the mutable accumulator behind a Metric. It is guarded by the
// Collector's mutex rather than carrying its own, because a signal
// updates the metric and the totals together and one lock is cheaper than
// two.
type metric = Metric

// newMetric starts an accumulator for a signal name.
func newMetric(name string, src Source) *metric {
	return &Metric{Name: name, Source: src}
}

// observe folds one signal into the accumulator.
func (m *Metric) observe(s Signal) {
	m.Count++
	if s.Failed {
		m.Failed++
	}
	if s.Cancelled {
		m.Cancelled++
	}
	if s.Async {
		m.Async++
	}
	m.Children += uint64(s.Children)
	m.Executed += uint64(s.Executed)

	d := s.Duration
	m.Total += d
	if m.Count == 1 || d < m.Min {
		m.Min = d
	}
	if d > m.Max {
		m.Max = d
	}
	m.Avg = m.Total / time.Duration(m.Count)

	m.TotalMS = ms(m.Total)
	m.MinMS = ms(m.Min)
	m.MaxMS = ms(m.Max)
	m.AvgMS = ms(m.Avg)

	if m.First.IsZero() || s.Time.Before(m.First) {
		m.First = s.Time
	}
	if s.Time.After(m.Last) {
		m.Last = s.Time
	}
	if m.Source == "" {
		m.Source = s.Source
	}
}

// ms converts a duration to fractional milliseconds.
func ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
