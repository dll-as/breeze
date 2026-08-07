package observability

import (
	"sort"
	"time"
)

// indexKey identifies a node in the signal graph.
func indexKey(src Source, name string) string { return string(src) + "\x00" + name }

// indexedNode accumulates what the collector knows about one signal name,
// including which child units have been seen running under it. It backs
// the graph view, which answers "when this fires, what actually runs?".
type indexedNode struct {
	Source Source
	Name   string

	Count uint64
	Last  time.Time

	// children maps a child unit's name to how often it has been seen.
	children map[string]*childStat
}

type childStat struct {
	count    uint64
	failed   uint64
	skipped  uint64
	total    time.Duration
	priority int
	phase    string
}

// observe folds a signal into the node.
func (n *indexedNode) observe(s Signal) {
	n.Count++
	if s.Time.After(n.Last) {
		n.Last = s.Time
	}
	if len(s.Spans) == 0 {
		return
	}
	if n.children == nil {
		n.children = make(map[string]*childStat, len(s.Spans))
	}
	for _, sp := range s.Spans {
		cs := n.children[sp.Name]
		if cs == nil {
			cs = &childStat{}
			n.children[sp.Name] = cs
		}
		cs.count++
		cs.total += sp.Duration
		cs.priority = sp.Priority
		cs.phase = sp.Phase
		if sp.Failed || sp.Panicked {
			cs.failed++
		}
		if sp.Skipped {
			cs.skipped++
		}
	}
}

// GraphNode is one vertex of the signal graph: something that fires.
type GraphNode struct {
	ID     string `json:"id"`
	Source Source `json:"source"`
	Name   string `json:"name"`
	Count  uint64 `json:"count"`

	// Last is when this node most recently fired.
	Last time.Time `json:"last"`

	// Edges are the child units that run when this node fires.
	Edges []GraphEdge `json:"edges"`
}

// GraphEdge links a node to a child unit that runs beneath it.
type GraphEdge struct {
	// Target is the child unit's name, e.g. a listener function.
	Target string `json:"target"`

	// Count is how many times the child has been observed.
	Count uint64 `json:"count"`

	// Failed and Skipped count the child's unhappy paths.
	Failed  uint64 `json:"failed"`
	Skipped uint64 `json:"skipped"`

	// AvgMS is the child's mean duration in milliseconds.
	AvgMS float64 `json:"avg_ms"`

	// Priority and Phase describe the child's ordering, when it has one.
	Priority int    `json:"priority"`
	Phase    string `json:"phase,omitempty"`
}

// Graph returns the observed signal graph: every name the collector has
// seen, and the child units that ran beneath it.
//
// The graph is built from observations rather than from registrations, so
// it shows what actually happens at runtime — a listener that is
// registered but always filtered out never appears.
func (c *Collector) Graph() []GraphNode {
	c.idxMu.RLock()
	out := make([]GraphNode, 0, len(c.index))
	for _, n := range c.index {
		node := GraphNode{
			ID:     indexKey(n.Source, n.Name),
			Source: n.Source,
			Name:   n.Name,
			Count:  n.Count,
			Last:   n.Last,
		}
		if len(n.children) > 0 {
			node.Edges = make([]GraphEdge, 0, len(n.children))
			for name, cs := range n.children {
				avg := time.Duration(0)
				if cs.count > 0 {
					avg = cs.total / time.Duration(cs.count)
				}
				node.Edges = append(node.Edges, GraphEdge{
					Target:   name,
					Count:    cs.count,
					Failed:   cs.failed,
					Skipped:  cs.skipped,
					AvgMS:    ms(avg),
					Priority: cs.priority,
					Phase:    cs.phase,
				})
			}
			// Sort by execution order so the graph renders the way the
			// dispatch actually ran: higher priority first.
			sort.Slice(node.Edges, func(i, j int) bool {
				if node.Edges[i].Priority != node.Edges[j].Priority {
					return node.Edges[i].Priority > node.Edges[j].Priority
				}
				return node.Edges[i].Target < node.Edges[j].Target
			})
		}
		out = append(out, node)
	}
	c.idxMu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns every distinct signal name the collector has observed,
// sorted alphabetically. It backs the dashboard's filter dropdown.
func (c *Collector) Names() []string {
	c.idxMu.RLock()
	out := make([]string, 0, len(c.index))
	for _, n := range c.index {
		out = append(out, n.Name)
	}
	c.idxMu.RUnlock()

	sort.Strings(out)
	return dedupe(out)
}

// dedupe removes adjacent duplicates from a sorted slice in place.
func dedupe(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
