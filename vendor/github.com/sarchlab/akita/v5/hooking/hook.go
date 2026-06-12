package hooking

// HookPos defines the enum of possible hooking positions.
type HookPos struct {
	Name string
}

// HookCtx is the context that holds all the information about the site that a
// hook is triggered.
type HookCtx struct {
	Domain Hookable
	Pos    *HookPos
	Item   interface{}
	Detail interface{}
}

// Hookable defines an object that accept Hooks.
type Hookable interface {
	// AcceptHook registers a hook.
	AcceptHook(hook Hook)

	// NumHooks returns the number of hooks registered.
	NumHooks() int

	// Hooks returns all the hooks registered.
	Hooks() []Hook
}

// Hook is a short piece of program that can be invoked by a hookable object.
type Hook interface {
	// Func determines what to do if hook is invoked.
	Func(ctx HookCtx)
}

// A HookableBase provides some utility function for other type that implement
// the Hookable interface.
type HookableBase struct {
	hookList []Hook
}

// NewHookableBase creates a HookableBase object.
func NewHookableBase() *HookableBase {
	h := new(HookableBase)
	h.hookList = make([]Hook, 0)

	return h
}

// NumHooks returns the number of hooks registered.
func (h *HookableBase) NumHooks() int {
	return len(h.hookList)
}

// Hooks returns all the hooks registered.
func (h *HookableBase) Hooks() []Hook {
	return h.hookList
}

// AcceptHook register a hook.
func (h *HookableBase) AcceptHook(hook Hook) {
	h.mustNotHaveDuplicatedHook(hook)
	h.hookList = append(h.hookList, hook)
}

func (h *HookableBase) mustNotHaveDuplicatedHook(hook Hook) {
	for _, h := range h.hookList {
		if h == hook {
			panic("duplicated hook")
		}
	}
}

// InvokeHook triggers the register Hooks.
func (h *HookableBase) InvokeHook(ctx HookCtx) {
	for _, hook := range h.hookList {
		hook.Func(ctx)
	}
}
