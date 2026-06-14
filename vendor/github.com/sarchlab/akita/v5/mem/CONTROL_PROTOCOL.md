# Memory Agent Control Protocol

A uniform request/response protocol that lets external code start, stop,
quiesce, and reset every memory agent in Akita — caches, TLBs, MMU,
GMMU, MMU cache, address translator, ROB, ideal memory controller,
DRAM, simple banked memory, and the datamover — through one message
type on one well-known port.

The protocol primitives live in `mem/protocol.go`. The reusable state
enum, support matrix, and a `*testing.T` conformance harness live in
`mem/memcontrolprotocol/` (see [`mem/memcontrolprotocol/README.md`](control/README.md)).

Every memory agent implements its supported subset of the protocol. The
support matrix and the [per-component behavior](#per-component-behavior)
below describe the implemented state.

## TL;DR

- Every memory agent exposes a port named `Control`.
- That port carries `memcontrolprotocol.Req` in and `memcontrolprotocol.Rsp` out (by
  value, not pointers — handlers type-assert `msg.(memcontrolprotocol.Req)`).
- The request's `Command` field is one of six verbs.
- The component runs the verb and replies on the same port.
- Whether the reply is same-tick (sync) or whenever-the-work-finishes
  (async) is fixed by the verb, not the component.
- A component that does not implement a verb still replies, with
  `Success: false, Error: "unsupported"`.

## The six verbs

Four verbs are **universal** — every memory agent supports them. Two
are **conditional** — only agents that hold private cache-of-memory
state support them.

| Verb           | Universal? | What it does                                                                                                                            | Ack timing |
| -------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------- | ---------- |
| **Pause**      | ✓          | Stop accepting new traffic and stop scheduling internal work. In-flight transactions stay where they are.                                | Sync       |
| **Drain**      | ✓          | Stop accepting new traffic; let in-flight transactions finish; end in the paused state.                                                  | Async      |
| **Enable**     | ✓          | Resume processing from paused.                                                                                                          | Sync       |
| **Reset**      | ✓          | Hard reset to post-build state: drop in-flight, clear queues, clear private caches, drain Top/Bottom queues. Legal from any state.       | Sync       |
| **Invalidate** | conditional | Drop entries from private cache state without writeback. Component must be paused or drained first. Filterable by `Addresses` and `PID`. | Sync       |
| **Flush**      | conditional | Write dirty private state back to backing memory. Clean entries stay valid. Component must be paused or drained first.                   | Async      |

### Sync vs async, more precisely

- **Sync** means the component sends the response in the same tick it
  receives the request. The caller sees `Success: true` (or
  `Success: false` for unsupported/illegal-state) on the very next
  outgoing message.
- **Async** means the request is accepted silently. The component
  acknowledges by sending the response when the underlying work is
  finished — Drain when all in-flight transactions have drained, Flush
  when all dirty data has been written back.

### Why no `PauseAfter`/`InvalidateAfter`/`DiscardInflight`

The old `Req` carried modifier flags that encoded compound
operations: "Flush, then pause", "Flush, then invalidate", "Flush, but
discard in-flight rather than wait for it." These are now sequenced by
the caller:

| Old compound                                                  | New sequence                          |
| ------------------------------------------------------------- | ------------------------------------- |
| `Flush{PauseAfter: true}`                                     | `Drain` → `Flush` (stays paused)      |
| `Flush{InvalidateAfter: true}`                                | `Drain` → `Flush` → `Invalidate`      |
| `Flush{DiscardInflight: true, InvalidateAfter: true}`         | `Pause` → `Reset`                     |

The primitives compose. The protocol stays small.

## Conventions

1. **One control port per component.** Every memory agent exposes a
   port named `Control`. It carries `memcontrolprotocol.Req` in and
   `memcontrolprotocol.Rsp` out (by value). Workload requests (reads, writes,
   translations, data-move requests) use other ports (`Top`,
   `Bottom`, `Inside`, `Outside`, etc.), never `Control`.
2. **One control state per component.** Every agent holds a
   `memcontrolprotocol.State` value in its own state struct. Values are
   `StateEnabled`, `StatePausing`, `StatePaused`, `StateDraining`,
   `StateFlushing`. Reset and Invalidate are operations within these
   states, not separate states; Reset always lands the component in
   `StateEnabled`.
3. **Unsupported verbs always reply.** A component that does not
   implement a verb sends `Rsp{Command: <verb>, Success:
   false, Error: memcontrolprotocol.ErrUnsupported}`. It never panics on a
   well-formed verb.
4. **Illegal-state verbs reply with a reason.** Invalidate and Flush
   require the component to be in `StatePaused` or `StateDraining`.
   Issuing them while `StateEnabled` returns `Success: false,
   Error: memcontrolprotocol.ErrMustBePausedOrDrained`.
5. **Control commands are processed serially.** A component handles one
   control command at a time, to completion, before it dequeues the next.
   While an async verb (Drain or Flush) is in progress the component does
   **not** accept another control command — the next command stays queued
   on the `Control` port and is taken only once the component settles
   (the async verb acks and the state lands in `StatePaused`), so each
   command is always handled from a clean, fully-settled state. There is
   no priority or preemption: a Reset queued behind an in-flight Drain
   waits for the Drain to finish and ack, then runs. Issuing a new control
   command before the previous one has acked is therefore safe — it is
   buffered, not dropped — but the sender must wait for each ack to know
   when the command took effect.
6. **Control is serviced before the data path.** Within a single tick a
   component handles control messages before advancing its data pipeline,
   so a synchronous verb (Pause/Reset) takes effect that same tick before
   any further Top/Bottom traffic or in-flight work advances.
7. **Verbs are idempotent.** Pause-when-Paused, Enable-when-Enabled,
   and Drain-when-Paused all succeed without side effects.

## Wire format

```go
type Command int

const (
    CmdPause Command = iota
    CmdDrain
    CmdEnable
    CmdReset
    CmdInvalidate
    CmdFlush
)

type Req struct {
    messaging.MsgMeta
    Command   Command
    Addresses []uint64 // Invalidate / Flush filter; empty = all entries.
    PID       vm.PID   // Invalidate / Flush filter; zero = all PIDs.
}

type Rsp struct {
    messaging.MsgMeta
    Command Command
    Success bool
    Error   string // Empty on success. memcontrolprotocol.ErrUnsupported or
                   // memcontrolprotocol.ErrMustBePausedOrDrained on failure.
}
```

`Addresses` and `PID` are only meaningful for `CmdInvalidate` and
`CmdFlush`. For the other verbs they are ignored.

## Support matrix (final state)

|                              | Pause | Drain | Enable | Reset | Invalidate | Flush |
| ---------------------------- | ----- | ----- | ------ | ----- | ---------- | ----- |
| `cache/writeback`            | ✓     | ✓     | ✓      | ✓     | ✓          | ✓     |
| `cache/writethroughcache`    | ✓     | ✓     | ✓      | ✓     | ✓          | no-op |
| `vm/tlb`                     | ✓     | ✓     | ✓      | ✓     | ✓          | —     |
| `vm/mmuCache`                | ✓     | ✓     | ✓      | ✓     | ✓          | —     |
| `vm/mmu`                     | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `vm/gmmu`                    | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `vm/addresstranslator`       | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `rob`                        | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `idealmemcontroller`         | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `dram`                       | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `simplebankedmemory`         | ✓     | ✓     | ✓      | ✓     | —          | —     |
| `datamover`                  | ✓     | ✓     | ✓      | ✓     | —          | —     |

Legend: **✓** = supported; **no-op** = supported (verb succeeds) but
does no work; **—** = unsupported (replies `Success: false,
Error: "unsupported"`).

The matrix corresponds directly to the `VerbSupport` each component
declares to `memcontrolprotocol.RunContract`:

- `cache/writeback`, `cache/writethroughcache` → `memcontrolprotocol.CacheLike()`
  (Universal + Invalidate + Flush).
- `vm/tlb`, `vm/mmuCache` → `memcontrolprotocol.TranslationCacheLike()`
  (Universal + Invalidate; Flush is **not** supported because
  translations are never dirty).
- everyone else → `memcontrolprotocol.Universal()` (Pause, Drain, Enable, Reset).

`mshr` is not in the matrix — it is a substructure of a cache, not a
component. Its state is part of the enclosing cache and is wiped by the
cache's Reset.

`Flush` on the writethrough cache is a *supported* verb (the matrix
declares it via `CacheLike()`), but it is a no-op because writethrough
holds no dirty data: it acks `Success: true` immediately. Callers can
therefore issue Flush uniformly across cache types without branching.

## Per-component behavior

This section records how each agent actually implements its verbs. The
universal verbs share a common shape everywhere, so it is stated once
here and only the component-specific details are repeated below:

- **Pause** (sync) sets the component to its paused state immediately;
  the data path stops accepting new traffic from its workload ports.
  In-flight work is frozen, not discarded.
- **Enable** (sync) returns the component to its running state and
  resumes processing. Traffic that queued while paused is processed once
  the data path runs again, not discarded.
- **Reset** (sync) is a hard reset: it discards in-flight transactions
  and internal queues and returns the component to its freshly-built
  shape. What exactly each component wipes (and deliberately preserves)
  is listed below.
- **Drain** (async) stops accepting new traffic, lets in-flight work
  finish, and acks once the component is quiescent — landing in the
  paused state. The per-component entry gives the exact quiescence
  condition.

Unsupported verbs reply `Success: false, Error: "unsupported"`.
Invalidate and Flush issued while running reply `Success: false,
Error: "must be paused or drained"`.

### Memory controllers

**`idealmemcontroller`** — `Universal`; state in `State.ControlState`
(`memcontrolprotocol.State`).
- Drain acks once `len(State.InflightTransactions) == 0`.
- Reset discards `InflightTransactions`.

**`dram`** — `Universal`; `State.ControlState`.
- Drain acks once `len(State.Transactions) == 0`.
- Reset clears `Transactions`, rebuilds the sub-transaction queue,
  command queues, and per-bank states, and resets the refresh counters.

**`simplebankedmemory`** — `Universal`; `State.ControlState`.
- Drain acks once every bank is quiescent (its pipeline is empty and its
  post-pipeline buffer is empty).
- Reset rebuilds all banks (fresh pipelines and buffers).

### Virtual-memory agents

**`vm/mmu`** — `Universal`; `State.ControlState`.
- Drain acks once `WalkingTranslations` is empty.
- Reset clears the in-flight walks and pending page-table-walker removals,
  and drains the Top port. The **shared page table** (owned by the
  simulation) is deliberately not touched.

**`vm/gmmu`** — `Universal`; `State.ControlState`.
- Drain acks once `WalkingTranslations` and `RemoteMemReqs` are empty.
- Reset clears in-flight walks and outstanding remote memory requests;
  the shared page table is not touched.

**`vm/addresstranslator`** — `Universal`; `State.ControlState`.
- Drain acks once `Transactions` and `InflightReqToBottom` are empty.
- Reset clears both and drains the Top/Bottom/Translation ports.

**`vm/tlb`** — `TranslationCacheLike` (Invalidate, no Flush); state is a
string in `State.TLBState` (`enable`/`pause`/`drain`).
- Drain lets the data path finish resolving in-flight misses (the MSHR
  empties and the final translation response is sent to the top) before
  the async ack.
- Reset is a hard reset to the freshly-built shape: it discards the MSHR
  (in-flight misses) and re-initializes the sets and pipeline, so cached
  translations and staged work are dropped.
- Invalidate marks cached entries matching the `Addresses`
  (page-aligned) and `PID` filter invalid (empty filter = all). No
  writeback — translations are never dirty.
- Flush is unsupported.

**`vm/mmuCache`** — `TranslationCacheLike`; string state in
`State.CurrentState`.
- Drain lets the data path forward/quiesce queued work, then acks.
- Reset drains the ports and re-initializes the cache table, returning it
  to its freshly-built empty state.
- Invalidate drops cached page-walk entries matching the filter. Because
  the cache stores per-level VPN segments, an address filter can also
  drop sibling pages that share an upper-level segment — this is safe
  over-invalidation (it only forces re-walks), never incorrect.
- Flush is unsupported.

### Caches

**`cache/writeback`** — `CacheLike`; state in `State.CacheState` (an int
holding `cacheState`: running / paused / draining / pre-flushing /
flushing).
- Drain acks once the cache is quiescent: no live (non-`Removed`)
  transactions, the write buffer is empty, and every per-bank in-flight
  counter is zero.
- Reset resets the directory and MSHR and clears all transactions,
  stage/bank buffers, pipelines, and flusher state.
- Invalidate drops blocks matching the `Addresses` (line-aligned) and
  `PID` filter **without writeback** — dirty data is discarded silently,
  so Flush first if it must survive.
- Flush (async) writes back the dirty blocks matching the filter and
  marks exactly those clean; clean lines and blocks outside the filter
  stay valid.

**`cache/writethroughcache`** — `CacheLike`; two bools `State.IsPaused`
and `State.IsDraining`.
- Drain acks once every transaction is retired (`Removed`).
- Reset resets the directory and MSHR and clears transactions and
  buffers.
- Invalidate drops matching blocks (always clean; no writeback).
- Flush is a no-op that acks `Success` immediately (no dirty data).

### Others

**`rob`** — `Universal`; `State.ControlState`. Pause freezes the
pipeline.
- Drain acks once `State.Transactions` is empty.
- Reset discards all in-flight transactions (releasing their receiver
  task IDs for tracing) and drains the Top/Bottom ports.

**`datamover`** — `Universal`; `State.ControlState`.
- Drain acks once the current transfer finishes
  (`State.CurrentTransaction.Active == false`).
- Reset wipes the current transaction and the data buffer and drains the
  Top/Inside/Outside ports.

## Helpers in `mem/memcontrolprotocol`

```go
import "github.com/sarchlab/akita/v5/mem/memcontrolprotocol"

// State enum used by every component for its control bookkeeping.
memcontrolprotocol.State            // StateEnabled, StatePausing, StatePaused, ...

// Per-component declaration of which verbs are supported.
memcontrolprotocol.VerbSupport{...}
memcontrolprotocol.Universal()                  // {Pause, Drain, Enable, Reset}
memcontrolprotocol.CacheLike()                  // Universal + Invalidate + Flush
memcontrolprotocol.TranslationCacheLike()       // Universal + Invalidate

// Verb classification.
memcontrolprotocol.IsSyncVerb(memcontrolprotocol.CmdPause)     // true

// Error string constants on Rsp.
memcontrolprotocol.ErrUnsupported
memcontrolprotocol.ErrMustBePausedOrDrained
```

## Implementing the protocol in a new component

1. Declare a `Control` port in the builder, and have setup code assign the
   instance after `Build`:
   ```go
   modelComp.DeclarePort("Control") // in Build

   // during assembly, after Build:
   comp.AssignPort("Control", modeling.MakePortBuilder().
       WithRegistrar(reg).
       WithComponent(comp).
       WithSpec(modeling.PortSpec{BufSize: ctrlBufSize}).
       Build("Control"))
   ```
2. Add a `memcontrolprotocol.State` field to the component's `State` struct so
   the control bookkeeping is uniform and serializable.
3. Add a control middleware that peeks the `Control` port, dispatches
   on `req.Command`, mutates `State.ControlState`, and sends the
   response per the sync/async timing rules.
4. For any verb you do not implement, reply with
   `Rsp{Success: false, Error: memcontrolprotocol.ErrUnsupported}`.
5. Declare the component's support matrix via a `VerbSupport` helper
   (`Universal()`, `CacheLike()`, or a literal).
6. Add one test that calls `memcontrolprotocol.RunContract`:
   ```go
   func TestControlContract(t *testing.T) {
       memcontrolprotocol.RunContract(t, "mycomp", buildMyComp,
           memcontrolprotocol.Universal())
   }
   ```
7. Add component-specific behavior tests separately — the contract
   harness only enforces the protocol surface (verb roundtrip, ack
   timing, supported/unsupported response shape). It does **not** check
   that Reset actually wiped the directory, that Flush actually wrote
   dirty data, etc. Those are component-internal invariants and belong
   in the component's own test file.

## Conformance harness: `memcontrolprotocol.RunContract`

```go
func RunContract(
    t *testing.T,
    name string,
    build memcontrolprotocol.BuildFunc, // func() *memcontrolprotocol.Harness
    matrix memcontrolprotocol.VerbSupport,
)

type Harness struct {
    Comp     Controllable      // Tick() bool, Name() string
    Ctrl     messaging.Port    // the component's Control port
    Teardown func()            // optional, called after each subtest
}
```

For each of the six verbs the harness:

- rebuilds the component fresh (verb tests are independent of each
  other),
- delivers a `Req` for that verb to `Ctrl`,
- ticks the component until a `Rsp` comes out (or a tick budget
  expires — 64 ticks for sync verbs and unsupported verbs, 4096 ticks
  for async verbs),
- asserts `Command`, `RspTo`, `Success`, and `Error` match the protocol
  for `(verb, supported?)`.

A failure in any verb is reported as a separate subtest, so the output
points at exactly which verb the component handles wrong.

## See also

- `mem/protocol.go` — the request/response type definitions.
- [`mem/memcontrolprotocol/README.md`](control/README.md) — the `control` package
  overview (State, VerbSupport, errors, `RunContract`).
- `mem/memcontrolprotocol/state.go` — `State` enum, `VerbSupport`, helpers.
- `mem/memcontrolprotocol/contract.go` — the `RunContract` harness.
