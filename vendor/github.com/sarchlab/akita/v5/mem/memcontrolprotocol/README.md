# mem/memcontrolprotocol

The protocol package for the uniform memory-agent control protocol. It
defines the protocol (`memcontrolprotocol.Protocol` with requester/responder roles),
its messages and verbs (`memcontrolprotocol.Req`, `memcontrolprotocol.Rsp`, `memcontrolprotocol.Command`),
the shared control-lifecycle types, and the conformance harness every
memory agent and its tests share.

See [`../CONTROL_PROTOCOL.md`](../CONTROL_PROTOCOL.md) for the full
protocol reference.

## What this package provides

- **`State`** — the control-lifecycle enum every memory agent stores in
  its own state struct: `StateEnabled`, `StatePausing`, `StatePaused`,
  `StateDraining`, `StateFlushing`. `String()` gives stable names for
  tracing.
- **`VerbSupport`** — a per-component declaration of which verbs are
  implemented, with the matrix constructors `Universal()`, `CacheLike()`,
  and `TranslationCacheLike()`, plus `Supports(cmd)`.
- **Error strings** — `ErrUnsupported` (`"unsupported"`) and
  `ErrMustBePausedOrDrained` (`"must be paused or drained"`), the
  conventional `Rsp.Error` values.
- **`IsSyncVerb(cmd)`** — reports whether a verb is acked synchronously.
- **`RunContract`** — a `*testing.T` harness that drives every verb
  against a built component over its real `Control` port and asserts the
  protocol response shape.

## The six verbs

Four are **universal** (every agent supports them); two are
**conditional** (only agents holding private cache-of-memory state).

| Verb       | Class       | Ack   |
| ---------- | ----------- | ----- |
| Pause      | universal   | sync  |
| Drain      | universal   | async |
| Enable     | universal   | sync  |
| Reset      | universal   | sync  |
| Invalidate | conditional | sync  |
| Flush      | conditional | async |

A **sync** verb is acked in the same tick it is received; an **async**
verb (Drain, Flush) is acked when the underlying work completes.

Invalidate and Flush are only legal once the component is paused or
drained. Issued while `StateEnabled`, they are rejected with
`Success: false, Error: ErrMustBePausedOrDrained`. Unsupported verbs
reply with `Success: false, Error: ErrUnsupported`.

## Support matrix

| Matrix                     | Components                                                                               | Extra verbs            |
| -------------------------- | --------------------------------------------------------------------------------------- | ---------------------- |
| `Universal()`              | dram, idealmemcontroller, simplebankedmemory, mmu, gmmu, addresstranslator, rob, datamover | —                      |
| `CacheLike()`              | cache/writeback, cache/writethroughcache                                                | Invalidate, Flush*     |
| `TranslationCacheLike()`   | vm/tlb, vm/mmuCache                                                                      | Invalidate             |

\* On `writethroughcache`, Flush is supported but a no-op (no dirty
data); it acks `Success: true` immediately.

## Adding the contract to a component

Each component package adds one test that builds the component and calls
`RunContract` with its declared matrix:

```go
func TestControlContract(t *testing.T) {
    build := func() *memcontrolprotocol.Harness {
        comp := MakeBuilder(). /* ... */ .Build("MyComp")
        return &memcontrolprotocol.Harness{
            Comp: comp,
            Ctrl: comp.GetPortByName("Control"),
        }
    }

    memcontrolprotocol.RunContract(t, "mycomp", build, memcontrolprotocol.Universal())
}
```

The harness rebuilds the component per verb, delivers each `Req`,
ticks until the `Rsp` arrives (or the tick budget expires), and
checks `Command`, `RspTo`, `Success`, and `Error`. It enforces only the
protocol surface — component-internal effects (directory cleared after
Reset, dirty data written back after Flush, etc.) belong in the
component's own behavior tests.
