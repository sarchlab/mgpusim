package tracing

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
)

// CollectTrace let the tracer to collect trace from a domain
func CollectTrace(domain NamedHookable, tracer Tracer) {
	hooks := domain.Hooks()
	for _, hook := range hooks {
		hook, ok := hook.(*traceHook)
		if ok && hook.t == tracer {
			panic(fmt.Sprintf(
				"domain %s already has tracer %s",
				domain.Name(), reflect.TypeOf(tracer)))
		}
	}

	h := traceHook{t: tracer}
	domain.AcceptHook(&h)
}

// A traceHook is a hook that traces tasks
type traceHook struct {
	t Tracer
}

// Func calls the tracer interfaces when the hook is triggered
func (h *traceHook) Func(ctx hooking.HookCtx) {
	switch ctx.Pos {
	case HookPosTaskStart:
		h.t.StartTask(ctx.Item.(Task))
	case HookPosTaskStep:
		h.t.StepTask(ctx.Item.(Task))
	case HookPosMilestone:
		h.t.AddMilestone(ctx.Item.(Milestone))
	case HookPosTaskEnd:
		h.t.EndTask(ctx.Item.(Task))
	}
}
