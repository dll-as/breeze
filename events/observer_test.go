package events

import (
	"errors"
	"sync"
	"testing"
)

// These tests exercise the observer hook from inside the events package,
// without depending on the observability layer. The contract they pin
// down is the one the hook promises: correct pairing of callbacks, total
// containment of observer misbehaviour, and no residual cost once
// detached.

type obsEvent struct{ ID int }

// recordingObserver captures every callback for assertion.
type recordingObserver struct {
	mu sync.Mutex

	starts    []DispatchInfo
	ends      []DispatchResult
	lstarts   []ListenerCall
	lends     []ListenerOutcome
	callOrder []string
}

func (o *recordingObserver) OnEventStart(d DispatchInfo) {
	o.mu.Lock()
	o.starts = append(o.starts, d)
	o.callOrder = append(o.callOrder, "start")
	o.mu.Unlock()
}

func (o *recordingObserver) OnEventEnd(d DispatchResult) {
	o.mu.Lock()
	o.ends = append(o.ends, d)
	o.callOrder = append(o.callOrder, "end")
	o.mu.Unlock()
}

func (o *recordingObserver) OnListenerStart(c ListenerCall) {
	o.mu.Lock()
	o.lstarts = append(o.lstarts, c)
	o.callOrder = append(o.callOrder, "lstart:"+c.ListenerName)
	o.mu.Unlock()
}

func (o *recordingObserver) OnListenerEnd(c ListenerOutcome) {
	o.mu.Lock()
	o.lends = append(o.lends, c)
	o.callOrder = append(o.callOrder, "lend:"+c.ListenerName)
	o.mu.Unlock()
}

func (o *recordingObserver) snapshot() ([]DispatchInfo, []DispatchResult, []ListenerCall, []ListenerOutcome) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]DispatchInfo(nil), o.starts...),
		append([]DispatchResult(nil), o.ends...),
		append([]ListenerCall(nil), o.lstarts...),
		append([]ListenerOutcome(nil), o.lends...)
}

func (o *recordingObserver) order() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.callOrder...)
}

// ─── Attach / detach ─────────────────────────────────────────────────────

func TestObserverAttachAndDetach(t *testing.T) {
	bus := New()
	defer bus.Close()

	if bus.ObserverEnabled() {
		t.Error("a fresh bus reports an observer")
	}
	if bus.Observer() != nil {
		t.Error("Observer() non-nil on a fresh bus")
	}

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	if !bus.ObserverEnabled() {
		t.Error("ObserverEnabled = false after SetObserver")
	}
	if bus.Observer() != obs {
		t.Error("Observer() did not return the attached observer")
	}

	bus.SetObserver(nil)
	if bus.ObserverEnabled() {
		t.Error("ObserverEnabled = true after detach")
	}
	if bus.Observer() != nil {
		t.Error("Observer() non-nil after detach")
	}
}

func TestObserverReplacement(t *testing.T) {
	bus := New()
	defer bus.Close()

	first := &recordingObserver{}
	second := &recordingObserver{}
	bus.SetObserver(first)
	bus.SetObserver(second)

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })
	EmitBus(bus, obsEvent{})

	if s, _, _, _ := first.snapshot(); len(s) != 0 {
		t.Error("the replaced observer still received callbacks")
	}
	if s, _, _, _ := second.snapshot(); len(s) != 1 {
		t.Errorf("the current observer received %d starts, want 1", len(s))
	}
}

func TestObserverSeesNothingAfterDetach(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })

	EmitBus(bus, obsEvent{})
	bus.SetObserver(nil)
	EmitBus(bus, obsEvent{})

	starts, _, _, _ := obs.snapshot()
	if len(starts) != 1 {
		t.Errorf("observer saw %d dispatches, want only the one before detach", len(starts))
	}
}

// ─── Callback pairing ────────────────────────────────────────────────────

func TestObserverCallbackOrder(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil }).Named("a")
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil }).Named("b")

	EmitBus(bus, obsEvent{})

	want := []string{"start", "lstart:a", "lend:a", "lstart:b", "lend:b", "end"}
	got := obs.order()
	if len(got) != len(want) {
		t.Fatalf("callback order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("callback order = %v, want %v", got, want)
		}
	}
}

func TestObserverDispatchInfo(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)
	Name[obsEvent](bus, "obs.event")
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })

	EmitBus(bus, obsEvent{ID: 3})

	starts, ends, _, _ := obs.snapshot()
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("got %d starts and %d ends, want 1 each", len(starts), len(ends))
	}

	s, e := starts[0], ends[0]
	if s.EventName != "obs.event" {
		t.Errorf("start EventName = %q, want obs.event", s.EventName)
	}
	if s.EventID == 0 {
		t.Error("start EventID is zero")
	}
	if s.EventID != e.EventID {
		t.Errorf("start id %d != end id %d", s.EventID, e.EventID)
	}
	if s.ListenerCount != 1 {
		t.Errorf("ListenerCount = %d, want 1", s.ListenerCount)
	}
	if s.PayloadSize == 0 {
		t.Error("PayloadSize not reported")
	}
	if s.Time.IsZero() {
		t.Error("start Time not set")
	}
	if e.ListenersExecuted != 1 {
		t.Errorf("ListenersExecuted = %d, want 1", e.ListenersExecuted)
	}
	if e.Async {
		t.Error("a synchronous dispatch was reported as async")
	}
}

func TestObserverListenerIdentity(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil }).
		Named("high").Priority(100)
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil }).
		Named("low").Priority(1)

	EmitBus(bus, obsEvent{})

	_, _, _, lends := obs.snapshot()
	if len(lends) != 2 {
		t.Fatalf("got %d listener outcomes, want 2", len(lends))
	}

	// Outcomes must carry ordering data, not just timing: the dashboard
	// reconstructs execution order from these fields alone.
	if lends[0].ListenerName != "high" || lends[0].Priority != 100 {
		t.Errorf("first outcome = %q/%d, want high/100", lends[0].ListenerName, lends[0].Priority)
	}
	if lends[0].Index != 0 || lends[1].Index != 1 {
		t.Errorf("indices = %d,%d, want 0,1", lends[0].Index, lends[1].Index)
	}
	if lends[0].Phase == "" {
		t.Error("Phase not reported")
	}
	if lends[0].ListenerID == 0 {
		t.Error("ListenerID not reported")
	}
}

func TestObserverSeesSkippedListener(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error {
		t.Error("filtered listener ran")
		return nil
	}).Named("filtered").Where(func(e obsEvent) bool { return e.ID > 100 })

	EmitBus(bus, obsEvent{ID: 1})

	_, ends, lstarts, lends := obs.snapshot()
	// Starts and ends must still pair up even for a skipped listener,
	// otherwise an observer cannot balance its bookkeeping.
	if len(lstarts) != 1 || len(lends) != 1 {
		t.Fatalf("starts/ends = %d/%d, want 1/1", len(lstarts), len(lends))
	}
	if !lends[0].Skipped {
		t.Error("Skipped = false for a filtered listener")
	}
	if ends[0].ListenersExecuted != 0 {
		t.Errorf("ListenersExecuted = %d, want 0", ends[0].ListenersExecuted)
	}
}

func TestObserverSeesError(t *testing.T) {
	bus := New(Config{OnError: func(*Context, string, error) {}})
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	boom := errors.New("boom")
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return boom }).Named("bad")

	EmitBus(bus, obsEvent{})

	_, ends, _, lends := obs.snapshot()
	if !errors.Is(lends[0].Err, boom) {
		t.Errorf("listener outcome Err = %v, want boom", lends[0].Err)
	}
	if !errors.Is(ends[0].Err, boom) {
		t.Errorf("dispatch Err = %v, want boom", ends[0].Err)
	}
}

func TestObserverSeesStop(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return Stop }).Named("stopper")
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error {
		t.Error("listener after Stop ran")
		return nil
	})

	EmitBus(bus, obsEvent{})

	_, ends, _, lends := obs.snapshot()
	if !errors.Is(lends[0].Err, Stop) {
		t.Errorf("outcome Err = %v, want Stop", lends[0].Err)
	}
	if !ends[0].Cancelled {
		t.Error("dispatch not reported as cancelled")
	}
	// Stop is control flow: it must not surface as a dispatch error.
	if ends[0].Err != nil {
		t.Errorf("dispatch Err = %v, want nil for a stopped dispatch", ends[0].Err)
	}
}

func TestObserverSeesPanic(t *testing.T) {
	bus := New(Config{OnPanic: func(*PanicError) {}})
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error {
		panic("listener boom")
	}).Named("panicky")

	EmitBus(bus, obsEvent{})

	_, _, _, lends := obs.snapshot()
	if len(lends) != 1 {
		t.Fatalf("got %d outcomes, want 1", len(lends))
	}
	if !lends[0].Panicked {
		t.Error("Panicked = false for a panicking listener")
	}
}

func TestObserverSeesAsyncDispatch(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	var wg sync.WaitGroup
	wg.Add(1)
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error {
		defer wg.Done()
		return nil
	}).Named("async")

	EmitAsyncWaitBus(bus, obsEvent{})
	wg.Wait()

	starts, ends, _, _ := obs.snapshot()
	if len(starts) == 0 || !starts[0].Async {
		t.Error("async dispatch not flagged on DispatchInfo")
	}
	if len(ends) == 0 || !ends[0].Async {
		t.Error("async dispatch not flagged on DispatchResult")
	}
}

// ─── Payload capture ─────────────────────────────────────────────────────

func TestObserverPayloadOptIn(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })

	EmitBus(bus, obsEvent{ID: 9})
	_, ends, _, _ := obs.snapshot()
	if ends[0].Payload != nil {
		t.Error("payload delivered without opting in")
	}

	bus.SetObserverWithPayload(obs)
	EmitBus(bus, obsEvent{ID: 9})

	_, ends, _, _ = obs.snapshot()
	last := ends[len(ends)-1]
	if last.Payload == nil {
		t.Fatal("payload not delivered after opting in")
	}
	got, ok := last.Payload.(obsEvent)
	if !ok {
		t.Fatalf("payload type = %T, want obsEvent", last.Payload)
	}
	if got.ID != 9 {
		t.Errorf("payload ID = %d, want 9", got.ID)
	}
}

func TestObserverPayloadClearedOnDetach(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserverWithPayload(obs)
	bus.SetObserver(nil)
	bus.SetObserver(obs) // reattached without payload

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })
	EmitBus(bus, obsEvent{ID: 1})

	_, ends, _, _ := obs.snapshot()
	if ends[0].Payload != nil {
		t.Error("payload preference survived a detach")
	}
}

// ─── Containment ─────────────────────────────────────────────────────────

// brokenObserver panics in every callback.
type brokenObserver struct{}

func (brokenObserver) OnEventStart(DispatchInfo)     { panic("start") }
func (brokenObserver) OnEventEnd(DispatchResult)     { panic("end") }
func (brokenObserver) OnListenerStart(ListenerCall)  { panic("lstart") }
func (brokenObserver) OnListenerEnd(ListenerOutcome) { panic("lend") }

func TestObserverPanicsAreContained(t *testing.T) {
	var panics []string
	var mu sync.Mutex
	bus := New(Config{
		OnPanic: func(p *PanicError) {
			mu.Lock()
			panics = append(panics, p.Listener)
			mu.Unlock()
		},
	})
	defer bus.Close()

	bus.SetObserver(brokenObserver{})

	ran := false
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error {
		ran = true
		return nil
	})

	// Every callback panics, yet the dispatch must still succeed.
	if err := EmitBus(bus, obsEvent{}); err != nil {
		t.Errorf("emit = %v, want nil despite observer panics", err)
	}
	if !ran {
		t.Error("listener did not run")
	}

	mu.Lock()
	n := len(panics)
	attributed := n > 0 && panics[0] == "<observer>"
	mu.Unlock()

	if n != 4 {
		t.Errorf("recovered %d observer panics, want 4", n)
	}
	if !attributed {
		t.Error("observer panic was not attributed to <observer>")
	}
}

func TestObserverPanicDoesNotCorruptMetrics(t *testing.T) {
	bus := New(Config{
		Metrics: true,
		OnPanic: func(*PanicError) {},
	})
	defer bus.Close()

	bus.SetObserver(brokenObserver{})
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })

	EmitBus(bus, obsEvent{})

	// A panicking observer must not be charged to the listener's panic
	// counter, or every dispatch would look broken on the dashboard.
	m := Inspect[obsEvent](bus).Metrics
	if m.Panics != 0 {
		t.Errorf("Panics = %d, want 0: observer panics are not listener panics", m.Panics)
	}
	if m.Dispatches != 1 {
		t.Errorf("Dispatches = %d, want 1", m.Dispatches)
	}
}

// ─── Concurrency ─────────────────────────────────────────────────────────

func TestObserverConcurrentSwap(t *testing.T) {
	bus := New()
	defer bus.Close()

	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })

	var wg sync.WaitGroup

	// Swapping the observer while dispatches are in flight is exactly
	// what the atomic pointer is for.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if i%2 == 0 {
				bus.SetObserver(&recordingObserver{})
			} else {
				bus.SetObserver(nil)
			}
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				EmitBus(bus, obsEvent{ID: j})
			}
		}()
	}

	wg.Wait()
}

func TestObserverConcurrentDispatch(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)
	OnTypeBus[obsEvent](bus, func(ctx *Context, e obsEvent) error { return nil })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				EmitBus(bus, obsEvent{ID: j})
			}
		}()
	}
	wg.Wait()

	starts, ends, _, _ := obs.snapshot()
	if len(starts) != 800 {
		t.Errorf("saw %d starts, want 800", len(starts))
	}
	if len(ends) != 800 {
		t.Errorf("saw %d ends, want 800", len(ends))
	}

	// Every dispatch id must be distinct, or correlation breaks.
	seen := make(map[uint64]bool, len(starts))
	for _, s := range starts {
		if seen[s.EventID] {
			t.Fatalf("duplicate dispatch id %d", s.EventID)
		}
		seen[s.EventID] = true
	}
}

// ─── No listeners ────────────────────────────────────────────────────────

func TestObserverSeesEventWithNoListeners(t *testing.T) {
	bus := New()
	defer bus.Close()

	obs := &recordingObserver{}
	bus.SetObserver(obs)

	// An event nobody listens to is still worth reporting: a tracer needs
	// to know it was emitted at all.
	EmitBus(bus, obsEvent{ID: 1})

	starts, ends, _, _ := obs.snapshot()
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("got %d starts and %d ends, want 1 each", len(starts), len(ends))
	}
	if starts[0].ListenerCount != 0 {
		t.Errorf("ListenerCount = %d, want 0", starts[0].ListenerCount)
	}
	if ends[0].ListenersExecuted != 0 {
		t.Errorf("ListenersExecuted = %d, want 0", ends[0].ListenersExecuted)
	}
}
