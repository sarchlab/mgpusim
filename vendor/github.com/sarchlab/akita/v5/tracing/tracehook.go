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
		h.t.StartTask(mustItem[TaskStart](ctx))
	case HookPosTaskTag:
		h.t.AddTaskTag(mustItem[TaskTag](ctx))
	case HookPosMilestone:
		h.t.AddMilestone(mustItem[Milestone](ctx))
	case HookPosTaskEnd:
		h.t.EndTask(mustItem[TaskEnd](ctx))
	}
}

// mustItem extracts the hook item as type T, panicking with a clear message
// that names the hook position and the actual type if the assertion fails.
func mustItem[T any](ctx hooking.HookCtx) T {
	item, ok := ctx.Item.(T)
	if !ok {
		var want T
		panic(fmt.Sprintf(
			"tracing: hook at %s carried %T, expected %T",
			ctx.Pos.Name, ctx.Item, want))
	}

	return item
}
