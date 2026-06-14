package memcontrolprotocol

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// Controllable is the minimal interface the contract harness requires
// from a component under test. Every Akita memory agent satisfies it
// through embedding modeling.TickingComponent.
type Controllable interface {
	Tick() bool
	Name() string
}

// Harness bundles a built component, its Control port, and a teardown
// callback. Build functions passed to RunContract return a *Harness.
type Harness struct {
	// Comp is the component under test. RunContract drives it via Tick.
	Comp Controllable

	// Ctrl is the component's Control port. RunContract delivers
	// ControlReq into it via Deliver and reads ControlRsp out of it via
	// RetrieveOutgoing.
	Ctrl messaging.Port

	// IsQuiescent, if non-nil, reports whether the component currently
	// holds no in-flight work. RunContract uses it to enforce that a Drain
	// ack and a Reset both leave the component quiescent. Components for
	// which quiescence is not a meaningful/cheap query may leave it nil to
	// skip the check.
	IsQuiescent func() bool

	// Teardown is invoked once after the harness finishes with this
	// component, regardless of test outcome. Nil is acceptable.
	Teardown func()
}

// BuildFunc constructs a fresh harness. The harness is rebuilt for each
// subtest so a verb cannot observe state left behind by a previous verb.
type BuildFunc func() *Harness

// maxTicks is the upper bound on how many ticks a sync verb (including
// the unsupported-verb response) is allowed to take before the harness
// gives up. The async verbs (Drain, Flush) use asyncMaxTicks.
const (
	maxTicks      = 64
	asyncMaxTicks = 4096
)

// RunContract exercises every verb in the protocol against the
// component produced by build. It uses t.Run so per-verb failures are
// reported independently.
//
// For every verb the harness:
//
//   - rebuilds the component (so verb tests are independent),
//   - delivers a ControlReq with that verb to the component's Control
//     port,
//   - drives Tick until a ControlRsp comes back (or a tick budget is
//     exhausted),
//   - asserts the Rsp matches the expected protocol response for
//     (verb, support-state).
//
// The expected response per verb is:
//
//   - Verb supported and synchronous: Rsp arrives within maxTicks with
//     Success=true and matching Command.
//   - Verb supported and asynchronous: Rsp arrives within asyncMaxTicks
//     with Success=true and matching Command.
//   - Verb unsupported: Rsp arrives within maxTicks with Success=false
//     and Error=ErrUnsupported.
//
// State-side assertions (e.g. directory cleared after Reset, transactions
// drained after Drain) live in per-component behavior tests, not here.
// This harness only enforces the protocol surface.
func RunContract(
	t *testing.T,
	name string,
	build BuildFunc,
	matrix VerbSupport,
) {
	t.Helper()

	verbs := []struct {
		cmd   Command
		label string
	}{
		{CmdPause, "Pause"},
		{CmdDrain, "Drain"},
		{CmdEnable, "Enable"},
		{CmdReset, "Reset"},
		{CmdInvalidate, "Invalidate"},
		{CmdFlush, "Flush"},
	}

	for _, v := range verbs {
		t.Run(fmt.Sprintf("%s/%s", name, v.label), func(t *testing.T) {
			h := build()
			defer func() {
				if h.Teardown != nil {
					h.Teardown()
				}
			}()

			checkVerb(t, h, v.cmd, matrix.Supports(v.cmd))
		})
	}

	// Conditional verbs (Invalidate, Flush) are only legal once the
	// component is paused or drained. When supported, issuing them from
	// the freshly-built (Enabled) state must be rejected with
	// ErrMustBePausedOrDrained rather than silently acted on.
	for _, v := range verbs {
		if !matrix.Supports(v.cmd) || !isConditionalVerb(v.cmd) {
			continue
		}

		t.Run(fmt.Sprintf("%s/%s-illegal-when-enabled", name, v.label),
			func(t *testing.T) {
				h := build()
				defer func() {
					if h.Teardown != nil {
						h.Teardown()
					}
				}()

				checkConditionalIllegalState(t, h, v.cmd)
			})
	}

	runLifecycleInvariants(t, name, build, matrix)
}

// runLifecycleInvariants checks properties that single-verb round trips
// miss: the sync universal verbs are idempotent, and Reset from the paused
// state lands the component back in a clean (quiescent) enabled state.
func runLifecycleInvariants(
	t *testing.T,
	name string,
	build BuildFunc,
	matrix VerbSupport,
) {
	t.Helper()

	if matrix.Pause {
		t.Run(name+"/idempotent-pause", func(t *testing.T) {
			h := build()
			defer teardown(h)
			driveExpectSuccess(t, h, CmdPause)
			driveExpectSuccess(t, h, CmdPause)
		})
	}

	if matrix.Enable {
		t.Run(name+"/idempotent-enable", func(t *testing.T) {
			h := build()
			defer teardown(h)
			driveExpectSuccess(t, h, CmdEnable)
			driveExpectSuccess(t, h, CmdEnable)
		})
	}

	if matrix.Reset && matrix.Pause {
		t.Run(name+"/reset-from-paused", func(t *testing.T) {
			h := build()
			defer teardown(h)
			driveExpectSuccess(t, h, CmdPause)
			driveExpectSuccess(t, h, CmdReset)
			if h.IsQuiescent != nil && !h.IsQuiescent() {
				t.Errorf("component not quiescent after Reset from Paused")
			}
		})
	}
}

func teardown(h *Harness) {
	if h.Teardown != nil {
		h.Teardown()
	}
}

// driveExpectSuccess delivers one verb and asserts a Success ack returns
// within the verb's tick budget.
func driveExpectSuccess(t *testing.T, h *Harness, cmd Command) {
	t.Helper()

	req := newControlReq(h.Ctrl, cmd)
	h.Ctrl.Deliver(req)

	budget := maxTicks
	if !IsSyncVerb(cmd) {
		budget = asyncMaxTicks
	}

	rsp, ok := drainForRsp(h, budget)
	if !ok {
		t.Fatalf("no ack for %v", cmd)
	}
	if rsp.Command != cmd || !rsp.Success {
		t.Errorf("%v: got %+v, want Success ack", cmd, rsp)
	}
}

// isConditionalVerb reports whether the verb requires the component to be
// paused or drained before it is legal.
func isConditionalVerb(cmd Command) bool {
	return cmd == CmdInvalidate || cmd == CmdFlush
}

// pauseForConditionalVerb drives the component to a paused state so a
// subsequent Invalidate or Flush is legal.
func pauseForConditionalVerb(t *testing.T, h *Harness) {
	t.Helper()

	req := newControlReq(h.Ctrl, CmdPause)
	h.Ctrl.Deliver(req)

	rsp, ok := drainForRsp(h, maxTicks)
	if !ok || rsp.Command != CmdPause || !rsp.Success {
		t.Fatalf("could not pause before conditional verb; got %+v", rsp)
	}
}

// checkConditionalIllegalState delivers a conditional verb to an Enabled
// component and asserts it is rejected per the protocol.
func checkConditionalIllegalState(
	t *testing.T,
	h *Harness,
	cmd Command,
) {
	t.Helper()

	req := newControlReq(h.Ctrl, cmd)
	h.Ctrl.Deliver(req)

	rsp, ok := drainForRsp(h, maxTicks)
	if !ok {
		t.Fatalf("no ControlRsp received for %v issued while Enabled", cmd)
	}

	if rsp.Command != cmd {
		t.Errorf("Rsp.Command = %v, want %v", rsp.Command, cmd)
	}

	if rsp.Success {
		t.Errorf("Rsp.Success = true, want false for %v while Enabled", cmd)
	}

	if rsp.Error != ErrMustBePausedOrDrained {
		t.Errorf("Rsp.Error = %q, want %q", rsp.Error, ErrMustBePausedOrDrained)
	}
}

// checkVerb performs one verb's round trip against the harness.
func checkVerb(
	t *testing.T,
	h *Harness,
	cmd Command,
	supported bool,
) {
	t.Helper()

	if supported && isConditionalVerb(cmd) {
		// Invalidate and Flush are only legal once the component is
		// paused or drained, so quiesce it before driving the verb.
		pauseForConditionalVerb(t, h)
	}

	req := newControlReq(h.Ctrl, cmd)
	h.Ctrl.Deliver(req)

	budget := maxTicks
	if supported && !IsSyncVerb(cmd) {
		budget = asyncMaxTicks
	}

	rsp, ok := drainForRsp(h, budget)
	if !ok {
		t.Fatalf("no ControlRsp received within %d ticks for verb %v "+
			"(supported=%v)", budget, cmd, supported)
	}

	if rsp.Command != cmd {
		t.Errorf("Rsp.Command = %v, want %v", rsp.Command, cmd)
	}

	if rsp.RspTo != req.ID {
		t.Errorf("Rsp.RspTo = %d, want %d", rsp.RspTo, req.ID)
	}

	if supported {
		if !rsp.Success {
			t.Errorf("Rsp.Success = false (Error=%q), want true",
				rsp.Error)
			return
		}
		// Drain promises quiescence on ack; Reset returns to a freshly-
		// built (quiescent) state. Enforce it when the component supplies
		// a probe.
		if h.IsQuiescent != nil &&
			(cmd == CmdDrain || cmd == CmdReset) &&
			!h.IsQuiescent() {
			t.Errorf("%v acked but component is not quiescent", cmd)
		}
		return
	}

	if rsp.Success {
		t.Errorf("Rsp.Success = true, want false for unsupported verb")
	}

	if rsp.Error != ErrUnsupported {
		t.Errorf("Rsp.Error = %q, want %q",
			rsp.Error, ErrUnsupported)
	}
}

// newControlReq builds a ControlReq addressed to the component's
// Control port from a fixed pseudo-source "ContractAgent".
func newControlReq(
	ctrl messaging.Port,
	cmd Command,
) Req {
	req := Req{Command: cmd}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("ContractAgent")
	req.Dst = ctrl.AsRemote()
	req.TrafficClass = "Req"
	return req
}

// drainForRsp ticks the component up to budget times waiting for a
// ControlRsp to appear on the Control port's outgoing queue. It returns
// the first such Rsp and true, or a zero Rsp and false if the budget is
// exhausted.
func drainForRsp(h *Harness, budget int) (Rsp, bool) {
	for range budget {
		if msg := h.Ctrl.RetrieveOutgoing(); msg != nil {
			if rsp, ok := msg.(Rsp); ok {
				return rsp, true
			}
		}
		h.Comp.Tick()
	}

	// One last sweep in case the final tick produced the Rsp.
	if msg := h.Ctrl.RetrieveOutgoing(); msg != nil {
		if rsp, ok := msg.(Rsp); ok {
			return rsp, true
		}
	}
	return Rsp{}, false
}
