# hooking — Generic Hook Mechanism

Package `hooking` provides the generic hook mechanism for the Akita simulation
framework. It defines the observer-style types that let any object expose
instrumentation points and let external code observe or react to what happens
at those points. It is a leaf package with no dependencies on other Akita
packages, so it is safe to import from any layer (engines, ports, components,
tracing).

## Key Concepts

- A **hook position** (`HookPos`) names a place in execution where hooks may
  fire, for example "before an event" or "a message was sent".
- A **hookable** object exposes such positions and lets hooks be registered
  against it.
- When the hookable reaches a position, it builds a **hook context**
  (`HookCtx`) and invokes every registered hook with it.

## Key Types

### HookPos and HookCtx

```go
type HookPos struct {
    Name string
}

type HookCtx struct {
    Domain Hookable    // the object firing the hook
    Pos    *HookPos    // where the hook fired
    Item   interface{} // the primary subject (e.g. the event or message)
    Detail interface{} // optional extra information
}
```

Packages declare their positions as package-level vars, e.g. `timing.HookPosBeforeEvent`
or `messaging.HookPosPortMsgSend`. A hook inspects `ctx.Pos` to decide whether
it cares about a given firing.

### Hook

```go
type Hook interface {
    Func(ctx HookCtx)
}
```

A hook is any value with a `Func` method that receives the context.

### Hookable

```go
type Hookable interface {
    AcceptHook(hook Hook) // register a hook
    NumHooks() int        // how many hooks are registered
    Hooks() []Hook        // a copy of the registered hooks
}
```

### HookableBase

Embed `HookableBase` to implement `Hookable` for free. In addition to the
interface methods it provides `InvokeHook(ctx)`, which the owner calls to fire
all registered hooks.

```go
type MyThing struct {
    hooking.HookableBase
}

func (t *MyThing) doWork() {
    // ... do something observable ...
    t.InvokeHook(hooking.HookCtx{
        Domain: t,
        Pos:    HookPosWorkDone,
        Item:   result,
    })
}
```

## How It Works

1. An object embeds `HookableBase` (or otherwise implements `Hookable`).
2. Observers register hooks with `obj.AcceptHook(hook)`. Registering the same
   hook twice panics with `"duplicated hook"`.
3. At each instrumentation point the object calls `InvokeHook(ctx)`, which
   runs every hook's `Func` in registration order.

```go
type logger struct{}

func (l *logger) Func(ctx hooking.HookCtx) {
    if ctx.Pos == HookPosWorkDone {
        log.Println("work done:", ctx.Item)
    }
}

thing.AcceptHook(&logger{})
```
