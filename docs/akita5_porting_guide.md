# MGPUSim → Akita v5.0.0-beta.2 Porting Guide

This document defines the conventions for porting MGPUSim from Akita v4 to v5.
It is the single source of truth during the port; follow it exactly so all
packages come out consistent. The local Akita v5 checkout is at
`/Users/yifan/dev/src/github.com/sarchlab/akita` (tag `v5.0.0-beta.2`).

## Scope decisions (already made)

- Components are rewritten in the idiomatic v5 style:
  `modeling.Component[Spec, State, Resources]` + middleware. Do NOT keep
  v4-style structs embedding `*sim.TickingComponent`.
- Page migration is dropped entirely (the `pagemigrationcontroller` package is
  already deleted). Remove migration-related messages, ports, and logic from
  driver/CP/protocol/configs.
- The v4 `analysis` package (PerfAnalyzer) integration is removed.
- The NVIDIA simulator is already deleted.
- Module path is now `github.com/sarchlab/mgpusim/v5`.

## Canonical reference files (read these before porting a component)

- `akita/examples/tickingping/{comp,builder,sendmw,receiveprocessmw,example_test}.go`
  — minimal full component pattern.
- `akita/mem/idealmemcontroller/{comp,builder,middleware,ctrlmiddleware}.go`
  — component with Resources, control middleware, multiple ports.
- `akita/examples/ping/` — event-driven component
  (`modeling.EventDrivenComponent`).
- `akita/doc/tutorial/migration.md` — official migration guide (some snippets
  reference stale pre-beta paths; the code is authoritative).
- `akita/examples/tasktree/main.go` — multi-component wiring + request tracing.

## Import path map (v4 → v5)

| v4 | v5 |
|---|---|
| `akita/v4/sim` (Msg, MsgMeta, Port, Connection, Component iface) | `akita/v5/messaging` |
| `akita/v4/sim` (Engine, Event, EventBase, Freq, VTimeInSec, IDGenerator, TimeTeller) | `akita/v5/timing` |
| `akita/v4/sim` (TickingComponent, Ticker, MiddlewareHolder, Domain) | `akita/v5/modeling` |
| `akita/v4/sim` (HookCtx, HookPos, Hookable, HookableBase) | `akita/v5/hooking` |
| `akita/v4/sim` (Buffer) | `akita/v5/queueing` (`Buffer[T]`) |
| `akita/v4/pipelining` | `akita/v5/queueing` (`Pipeline[T]`) |
| `akita/v4/sim/directconnection` | `akita/v5/noc/directconnection` |
| `akita/v4/mem/mem` | `akita/v5/mem` (Storage, AddressConverter, AddressToPortMapper, SinglePortMapper, InterleavedAddressPortMapper, BankedAddressPortMapper) |
| `akita/v4/mem/mem` (ReadReq, WriteReq, DataReadyRsp, WriteDoneRsp, AccessReq/Rsp) | `akita/v5/mem/memprotocol` |
| `akita/v4/mem/mem` (ControlMsg) and `mem/cache` (FlushReq/RestartReq), tlb flush/restart | `akita/v5/mem/memcontrolprotocol` (unified `Req`/`Rsp`) |
| `akita/v4/mem/vm` (PID, Page, PageTable) | `akita/v5/mem/vm` |
| `akita/v4/mem/vm` (TranslationReq/Rsp) | `akita/v5/mem/vm/vmprotocol` |
| `akita/v4/mem/vm/tlb` | `akita/v5/mem/vm/tlb` |
| `akita/v4/mem/vm/mmu` | `akita/v5/mem/vm/mmu` (page migration removed upstream) |
| `akita/v4/mem/vm/addresstranslator` | `akita/v5/mem/vm/addresstranslator` |
| `akita/v4/mem/idealmemcontroller` | `akita/v5/mem/idealmemcontroller` |
| `akita/v4/mem/cache/writeback` | `akita/v5/mem/cache/writeback` |
| `akita/v4/mem/cache/writethrough`, `writearound` | `akita/v5/mem/cache/writethroughcache` (Spec.WritePolicyType: `"write-through"` / `"write-around"` / `"write-evict"`) |
| `akita/v4/mem/dram` | `akita/v5/mem/dram` (rewritten; use Spec presets, e.g. HBM2Spec) |
| `akita/v4/noc/networking/pcie` | `akita/v5/noc/networking/pcie` |
| `akita/v4/tracing` | `akita/v5/tracing` (API changed, see below) |
| `akita/v4/monitoring` | `akita/v5/monitoring2` |
| `akita/v4/simulation` | `akita/v5/simulation` |
| `akita/v4/analysis` | deleted — remove usage |
| `akita/v4/sim` Domain | `modeling` has no Domain; restructure config code to plain structs holding components/ports |

## Component pattern

Each component package defines (usually in separate files):

```go
// comp.go — data types only, no behavior
type Spec struct {                 // immutable config; FLAT primitives only:
    Freq timing.Freq `json:"freq"` // bool/ints/uints/floats/string, slices of
    NumX int         `json:"num_x"`// primitives, maps str|int → primitive.
}                                  // NO nested structs, pointers, interfaces.

type State struct {                // mutable runtime data; primitives, nested
    ...                            // structs, slices, maps. NO pointers,
}                                  // interfaces, funcs, channels.

type Resources struct {            // shared refs (pointers OK, not validated,
    PageTable vm.PageTable         // not checkpointed). Use modeling.None as
}                                  // the third type param when empty.

type Comp = modeling.Component[Spec, State, Resources]
```

Validation is enforced at `Build()` time — a pointer in State panics. Where
MGPUSim runtime state is a complex object graph (wavefronts, in-flight maps
holding messages), keep it as fields on the middleware struct (not
checkpointable — acceptable for now and marked with a `// TODO(akita5):
state purity` comment) or in Resources if shared. Prefer pure State when the
conversion is cheap (counters, flags, queues of value types, RemotePort
strings, IDs).

```go
// builder.go
func MakeBuilder() Builder { return Builder{spec: defaultSpec} }
// Builder has WithRegistrar(modeling.Registrar), WithSpec(Spec),
// WithResources(Resources) (only if needed); all value-receiver chaining.

func (b Builder) Build(name string) *Comp {
    comp := modeling.NewBuilder[Spec, State, Resources]().
        WithEngine(b.registrar.GetEngine()).
        WithFreq(b.spec.Freq).
        WithSpec(b.spec).
        WithResources(b.resources).
        Build(name)
    comp.State = State{...}
    comp.AddMiddleware(&ctrlMW{comp: comp})   // order matters: control first
    comp.AddMiddleware(&xxxMW{comp: comp})
    comp.DeclarePort("Top", memprotocol.Responder) // roles optional; omit for
    comp.DeclarePort("Ctrl")                       // mgpusim-internal protocols
    b.registrar.RegisterComponent(comp)
    return comp
}
```

```go
// xxxmw.go — one middleware per concern; behavior lives here
type xxxMW struct {
    comp *Comp
    // non-checkpointed complex runtime state may live here (documented)
}
func (m *xxxMW) Tick() bool { ... }   // no time arg; m.comp.CurrentTime()
```

Ports are built EXTERNALLY (in the platform/config code or test setup), not
inside the component:

```go
p := modeling.MakePortBuilder().
    WithRegistrar(reg).WithComponent(comp).
    WithSpec(modeling.PortSpec{BufSize: 16}).
    Build("Top")              // creates "CompName.Top" and registers it
comp.AssignPort("Top", p)
```

In tests without a simulation, use `modeling.NewStandaloneRegistrar(engine)`
and/or `messaging.NewPort(comp, inCap, outCap, "Comp.Port")`.

Inside middleware, get ports with `m.comp.GetPortByName("Top")`. If a port is
fetched every tick, caching the lookup in the middleware struct is fine.

Naming rules (`akita/naming/naming.go`): hierarchical dot-separated CamelCase,
no underscores/dashes; indices in brackets (`GPU[1].CU[3]`); port names
globally unique as `Component.Port`.

## Messages

- Messages are VALUE types: embed `messaging.MsgMeta` by value; `Meta()` is
  provided by the embedded MsgMeta (returns value). Remove all `*XxxReq`
  pointers: construct with struct literals, pass by value, type-switch on
  value cases (`case protocol.MapWGReq:`).
- `MsgMeta` fields: `ID uint64`, `Src, Dst messaging.RemotePort`,
  `TrafficClass string`, `TrafficBytes int`, `RspTo uint64`.
- IDs come from `timing.GetIDGenerator().Generate()` (uint64). The "no ID"
  sentinel is `0`, not `""`.
- All v4 message Builders (e.g. `mem.ReadReqBuilder`) are gone. Example:

```go
req := memprotocol.ReadReq{
    MsgMeta: messaging.MsgMeta{
        ID:  timing.GetIDGenerator().Generate(),
        Src: m.port.AsRemote(),
        Dst: m.comp.State.MemDst,
    },
    Address:        addr,
    AccessByteSize: 64,
    PID:            pid,
}
```

- Responses link via `RspTo: req.ID` (uint64). `msg.Meta().IsRsp()` exists.
- MGPUSim's own protocols (cuprotocol, driverprotocol, rdma) keep their
  message types but converted to value style. General-response types like v4
  `sim.GeneralRsp` are gone — define small response structs in the protocol
  package where needed.
- A received `Msg` is an interface value; after `RetrieveIncoming()` /
  `PeekIncoming()` type-switch on concrete value types.

## Ports: send / receive

- `port.Send(msg)` returns NOTHING and panics if the outgoing buffer is full.
  ALWAYS guard: `if !port.CanSend() { return false }` then `port.Send(msg)`.
  The v4 pattern `if err := port.Send(...); err != nil` must be rewritten,
  not just compile-fixed.
- Receive: `PeekIncoming() messaging.Msg` (nil if empty), commit with
  `RetrieveIncoming()`. No time arguments anywhere.
- `port.AsRemote()` returns `messaging.RemotePort` (a string) — use for
  Src/Dst. Components store destination ports as `RemotePort`, never as
  `messaging.Port` references (except their own ports).
- Connections: `directconnection.MakeBuilder().WithRegistrar(reg).
  WithSpec(directconnection.Spec{Freq: 1 * timing.GHz}).Build(name)`;
  `conn.PlugIn(port)` — no buffer-size argument.

## Time, frequency, ticking

- `sim.VTimeInSec` (float64 seconds) → `timing.VTimeInPicoSec` (uint64
  picoseconds). For human-readable seconds (logs, reports, statistics):
  `float64(t) * 1e-12`.
- `sim.Freq` → `timing.Freq` (uint64 Hz): `1 * timing.GHz`. Helpers:
  `Period()`, `ThisTick(t)`, `NextTick(t)`, `NCyclesLater(n, t)`.
- `Tick(now sim.VTimeInSec) bool` → `Tick() bool`. Current time:
  `comp.CurrentTime()`.
- `TickNow()` / `TickLater()` take no arguments.
- Cycle-counting countdowns are preferred over scheduling ad-hoc events.

## Custom events (only where truly needed)

Events carry `HandlerID() string`; engines dispatch via a registry. The
component's own ticking handler is registered under the component name. For a
custom event (e.g. WfCompletionEvent):

- Keep a custom event type embedding `timing.EventBase`; create with
  `timing.MakeEventBase(time, handlerID)`.
- Register the handler:
  `engine.(timing.HandlerRegistrar).RegisterHandler("Comp.Name.EventKind", h)`
  during Build (cast is safe for both serial/parallel engines).
- Where the v4 code scheduled an event merely to delay work, prefer
  converting to a countdown inside Tick, or to
  `modeling.EventDrivenComponent` + `ScheduleWakeAt` for reactive components
  (see `akita/examples/ping`).

## Control protocol (CP ↔ caches/TLBs/AT/memory)

v4 per-component `cache.FlushReq`, `cache.RestartReq`, `tlb.FlushReq`,
`tlb.RestartReq`, `mem.ControlMsg`, AT/ROB flush-restart messages →
`memcontrolprotocol.Req{Command: CmdPause|CmdDrain|CmdEnable|CmdReset|
CmdInvalidate|CmdFlush, Addresses, PID}` / `memcontrolprotocol.Rsp{Command,
Success, Error}` sent to each component's `"Control"` port. v4
"flush + discard" maps to Pause/Drain + Invalidate (+ Flush for writeback);
"restart" maps to CmdReset or CmdEnable depending on intent (read
`akita/mem/CONTROL_PROTOCOL.md`). MGPUSim's own components that accept
control commands (ROB, AT, custom caches' wrappers, CU pipeline
flush/restart stays mgpusim-internal via cuprotocol) should implement a
`"Control"` port speaking memcontrolprotocol when they replace a v4
flush/restart flow.

## Tracing

- `tracing.StartTask(domain, tracing.TaskStart{ID, ParentID, Kind, What,
  Location, Detail})`, `tracing.EndTask(domain, tracing.TaskEnd{ID})`. IDs
  are uint64. Time is stamped inside the call from the domain clock.
- Request lifecycle helpers survive: `TraceReqInitiate`, `TraceReqReceive`,
  `TraceReqComplete`, `TraceReqFinalize`, and `tracing.MsgIDAtReceiver`.
  Check exact signatures in `akita/tracing/api.go` — req-trace helpers now
  take the message + domain and return/accept uint64 task IDs.
- Steps became `TaskTag`s; milestones: `tracing.AddMilestone(domain,
  tracing.Milestone{...})` with `MilestoneKind*` constants.
- `tracing.CollectTrace(comp, tracer)` unchanged. `NamedHookable` is now
  `naming.Named + hooking.Hookable + timing.TimeTeller + InvokeHook`.

## Hooks

`sim.HookCtx/HookPos/Hookable/HookableBase` → same names in `hooking`.
Port hook positions: `messaging.HookPosPortMsgSend/Recvd/RetrieveIncoming`.
Event positions: `timing.HookPosBeforeEvent/AfterEvent`.

## Buffers and pipelines

- `sim.Buffer` → `queueing.Buffer[T]`: `queueing.NewBuffer[T](name, cap)`;
  methods `CanPush/PushTyped/Pop/Peek/Size/Capacity`.
- `pipelining.Pipeline` → `queueing.Pipeline[T]`:
  `queueing.NewPipeline[T](width, numStages)`; `CanAccept/Accept/
  AcceptWithDelay/Tick(sink)/Clear`. Per-stage cycle count is gone — fold
  CyclePerStage into numStages. Pipeline items are typed values (no
  `pipelining.PipelineItem` interface; any T works).

## Simulation / platform assembly

- `simulation.MakeBuilder()` options: `WithParallelEngine()`,
  `WithoutMonitoring()`, `WithMonitorPort(n)`, `WithOutputFileName(s)`,
  `WithVisTracingOnStart()`. Accessors: `GetEngine() timing.Engine`,
  `GetDataRecorder()`, `GetMonitor() *monitoring2.Monitor`,
  `GetVisTracer()`, `GetComponentByName`, `GetPortByName`, `Terminate()`.
- `*simulation.Simulation` implements `modeling.Registrar` — pass it as the
  registrar to all builders. Ports register via the port builder.
- monitoring2: progress bars via `monitor.CreateProgressBar(name, total)`;
  component registration is automatic through the registrar.

## Mocks / tests

- Regenerate `mockgen` mocks: v4 `sim Port,Engine,Buffer` mocks →
  `messaging Port` + `timing Engine` (or use real
  `timing.NewSerialEngine()` + `modeling.NewStandaloneRegistrar`; prefer
  real objects over mocks where the test allows). `pipelining Pipeline`
  mocks → use real `queueing.Pipeline[T]` values.
- Update `//go:generate mockgen` directives to v5 packages before running
  `go generate ./...`.
- Ginkgo/Gomega versions stay as-is.

## Verification per package

```
cd <worktree-root>
go build ./<package>/...
go vet ./<package>/...
go test ./<package>/...   # once mocks/tests are ported
```

Packages must be ported bottom-up so `go build` stays meaningful:
protocol/kernels/sampling → emu → wavefront → timing/mem,rob,rdma →
cu → cp → driver → benchmarks(gputensor,mccl) → samples/runner+configs →
server, tests.
