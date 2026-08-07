package observability

import (
	"sort"
	"strings"
	"time"
)

// Query filters the retained signals. The zero Query matches everything.
//
// Filters combine with AND: a Query with both Name and Failed set matches
// signals that are both.
type Query struct {
	// Name matches the signal name exactly, when non-empty.
	Name string

	// Source matches the producing subsystem, when non-empty.
	Source Source

	// NameContains matches signal names containing this substring,
	// case-insensitively.
	NameContains string

	// Since and Until bound the time range. Zero values are open ends.
	Since time.Time
	Until time.Time

	// FailedOnly restricts results to failures.
	FailedOnly bool

	// SlowerThan restricts results to signals that took at least this
	// long. Zero means no lower bound.
	SlowerThan time.Duration

	// RequestID and CorrelationID match their respective fields exactly.
	RequestID     string
	CorrelationID string

	// Limit caps the number of results. Zero means no cap.
	Limit int

	// Newest returns the most recent matches rather than the oldest.
	Newest bool
}

// matches reports whether s satisfies the query.
func (q Query) matches(s Signal) bool {
	if q.Name != "" && s.Name != q.Name {
		return false
	}
	if q.Source != "" && s.Source != q.Source {
		return false
	}
	if q.NameContains != "" &&
		!strings.Contains(strings.ToLower(s.Name), strings.ToLower(q.NameContains)) {
		return false
	}
	if !q.Since.IsZero() && s.Time.Before(q.Since) {
		return false
	}
	if !q.Until.IsZero() && s.Time.After(q.Until) {
		return false
	}
	if q.FailedOnly && !s.Failed {
		return false
	}
	if q.SlowerThan > 0 && s.Duration < q.SlowerThan {
		return false
	}
	if q.RequestID != "" && s.RequestID != q.RequestID {
		return false
	}
	if q.CorrelationID != "" && s.CorrelationID != q.CorrelationID {
		return false
	}
	return true
}

// Find returns the retained signals matching q.
func (c *Collector) Find(q Query) []Signal {
	all := c.ring.Snapshot()

	out := make([]Signal, 0, min(len(all), max(q.Limit, 16)))
	if q.Newest {
		// Walk backwards so the limit keeps the most recent matches
		// rather than the oldest ones.
		for i := len(all) - 1; i >= 0; i-- {
			if !q.matches(all[i]) {
				continue
			}
			out = append(out, all[i])
			if q.Limit > 0 && len(out) >= q.Limit {
				break
			}
		}
		return out
	}

	for _, s := range all {
		if !q.matches(s) {
			continue
		}
		out = append(out, s)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out
}

// Recent returns the n most recent signals, newest first.
func (c *Collector) Recent(n int) []Signal {
	return c.Find(Query{Limit: n, Newest: true})
}

// ByID returns the retained signal with the given observability id.
func (c *Collector) ByID(id uint64) (Signal, bool) {
	all := c.ring.Snapshot()
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].ID == id {
			return all[i], true
		}
	}
	return Signal{}, false
}

// Slowest returns the n slowest retained signals, slowest first.
func (c *Collector) Slowest(n int) []Signal {
	all := c.ring.Snapshot()
	sort.Slice(all, func(i, j int) bool { return all[i].Duration > all[j].Duration })
	if n > 0 && len(all) > n {
		all = all[:n]
	}
	return all
}

// TopNames returns the n most frequent signal names, busiest first.
func (c *Collector) TopNames(n int) []Metric {
	all := c.Metrics()
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Name < all[j].Name
	})
	if n > 0 && len(all) > n {
		all = all[:n]
	}
	return all
}

// Rate returns the number of signals published in the trailing window,
// expressed per second.
//
// It is computed from the ring buffer, so a window longer than the
// retained history under-reports rather than misreports.
func (c *Collector) Rate(window time.Duration) float64 {
	if window <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-window)
	all := c.ring.Snapshot()

	n := 0
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Time.Before(cutoff) {
			break
		}
		n++
	}
	return float64(n) / window.Seconds()
}

// Clear discards the retained signals. Metrics and the graph are kept, so
// clearing the view does not erase the statistics.
func (c *Collector) Clear() { c.ring.Clear() }

// Reset discards the retained signals, the metrics and the graph.
func (c *Collector) Reset() {
	c.ring.Clear()

	c.mu.Lock()
	c.metrics = make(map[string]*metric)
	c.total = Stats{}
	c.mu.Unlock()

	c.idxMu.Lock()
	c.index = make(map[string]*indexedNode)
	c.childCounts = make(map[uint64]int)
	c.idxMu.Unlock()
}
