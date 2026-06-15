# Akita v5 port — cross-package notes

Accumulated as packages are ported. Two kinds of entries: (1) exported API
changes that dependent packages must adopt, (2) behavior deltas vs v4 that
matter for parity validation.

## amd/protocol (ported)

- All messages are value types; no builders/constructors. Construct with
  struct literals + `messaging.MsgMeta`.
- `WGCompletionMsg.RspTo []string` → `RspToIDs []uint64`.
- `LaunchKernelRsp.RspTo string` → use `MsgMeta.RspTo uint64`.
- Page-migration messages removed. `GeneralRsp` added (replaces v4
  `sim.GeneralRsp`).
- `StartTime`/`EndTime` fields removed from all messages (were never read).

## amd/kernels (ported)

- `WorkGroup.UID`, `Wavefront.UID`: `string` → `uint64`.
- `Wavefront.IssueTime/FinishTime`: `sim.VTimeInSec` →
  `timing.VTimeInPicoSec`.

## amd/sampling (ported)

- `Collect(issueTime, finishTime timing.VTimeInPicoSec)`;
  `Predict() (timing.VTimeInPicoSec, bool)`. Internals are float64 seconds.

## amd/timing/rdma (ported)

Dependents: timingconfig builders (r9nano, mi300a), CP ctrlMiddleware.

| Old (v4) | New (v5) |
|---|---|
| `rdma.DrainReqBuilder{}...Build()` | `rdma.DrainReq{MsgMeta: messaging.MsgMeta{ID: ..., Src: ..., Dst: ...}}` value |
| `rdma.RestartReqBuilder{}...Build()` | `rdma.RestartReq{...}` value |
| `case *rdma.DrainRsp:` / `case *rdma.RestartRsp:` | value cases; responses carry `RspTo` |
| `Builder.WithEngine/WithFreq/WithBufferSize/WithLocalModules/WithRemoteModules/...` | `WithRegistrar(reg)`, `WithSpec(rdma.Spec{...})` from `rdma.DefaultSpec()`, `WithResources(rdma.Resources{LocalModules, RemoteRDMAAddressTable})` |
| `comp.SetLocalModuleFinder(m)` / `comp.RemoteRDMAAddressTable = m` post-build | pass via `WithResources` before Build (mapper contents may be populated later) |
| Port fields `comp.RDMARequestInside` etc. | port names: `"RDMARequestInside"` (memprotocol.Responder), `"RDMARequestOutside"` (Requester), `"RDMADataInside"` (Requester), `"RDMADataOutside"` (Responder), `"Ctrl"`. Build externally, `AssignPort`, access via `GetPortByName` |

Behavior deltas (parity): per-cycle req/rsp limits now actually bind (v4 had
an unbounded inner forward loop); RestartReq retrieved only after its
response sends. Future cleanup: replace the mgpusim drain/restart protocol
with memcontrolprotocol on a "Control" port (CmdDrain/CmdEnable).

## amd/timing/wavefront (ported)

- `Inst.ID`: `string` → `uint64`.
- `Wavefront.LastFetchTime`: → `timing.VTimeInPicoSec`.
- `NewWfCompletionEvent(time timing.VTimeInPicoSec, handlerID string, wf)
  WfCompletionEvent` — value event; the CU must register the handler under
  that ID via `engine.(timing.HandlerRegistrar).RegisterHandler`.
- `WorkGroup.MapReq` is a value `protocol.MapWGReq`;
  `NewWorkGroup(raw, req protocol.MapWGReq)`.

## amd/timing/rob (ported)

Dependents: shaderarray config, CP ctrl flow.

| Old (v4) | New (v5) |
|---|---|
| `rob.MakeBuilder().WithEngine/WithFreq/WithBufferSize/WithNumReqPerCycle/WithBottomUnit` | `WithRegistrar(reg)`, `WithSpec(rob.Spec{Freq, BufferSize, NumReqPerCycle, BottomUnit})` from `rob.DefaultSpec()` |
| `*rob.ReorderBuffer` | `*rob.Comp` |
| post-build `rob.BottomUnit = port.AsRemote()` | `Spec.BottomUnit` before Build |
| builder auto-created ports | ports `"Top"/"Bottom"/"Control"` declared in Build; config creates externally + `AssignPort` |
| v4 flush `mem.ControlMsg{DiscardTransactions}` | `memcontrolprotocol.Req{Command: CmdPause}` at flush time |
| v4 restart `mem.ControlMsg{Restart}` | `memcontrolprotocol.Req{Command: CmdReset}` at restart time |

## amd/emu (ported)

Dependents: emusystem/emugpu config, CP resource pool.

- `emu.NewComputeUnit` / `BuildComputeUnit` / `BuildComputeUnitWithALU` /
  `ALUFactory` → removed. Assemble via: `NewStorageAccessor(...)` (same
  signature, v5 types) + `emu.NewALU(sa)` (or cdna3) +
  `emu.MakeBuilder().WithRegistrar(reg).WithSpec(emu.DefaultSpec()).
  WithResources(emu.Resources{Decoder, ALU, StorageAccessor}).Build(name)`,
  then build a port externally and `comp.AssignPort(emu.DispatchPortName, p)`.
- `*emu.ComputeUnit` → `*emu.Comp`
  (= `modeling.EventDrivenComponent[Spec, State, Resources]`).
- CU capability accessors (ControlPort/DispatchingPort/WfPoolSizes/...) →
  `emu.DispatcherView{CU: comp}` adapter; CP's resource pool registers the
  view.
- `cu.ToDispatcher` field → `comp.GetPortByName(emu.DispatchPortName)`.

## amd/timing/mem (ported)

Dependents: mi300a config (simplebankedmemory), shaderarray config (note:
shaderarray currently uses AKITA's addresstranslator, not mgpusim's — the
config porter chooses; mgpusim's adds TLB-request coalescing).

addresstranslator:
- Builder: `WithRegistrar(reg)` + `WithSpec(Spec{Freq, Log2PageSize,
  DeviceID, NumReqPerCycle})` from `DefaultSpec()`;
  `WithResources(Resources{MemProviderMapper, TranslationProviderMapper})`
  (wrap single ports in `&mem.SinglePortMapper{Port: p}`). String-kind
  convenience options removed.
- Ports `Top`/`Bottom`/`Translation`/`Control` declared in Build, assigned
  externally; `WithCtrlPort` removed — control arrives on "Control"
  (memcontrolprotocol). v4 flush → CmdPause+CmdReset; restart → CmdEnable.
- `Info` no longer propagated onto bottom-side requests.

simplebankedmemory (affects mi300a buildDRAMControllers):
- All per-field With* options → Spec fields (NumBanks, BankPipelineWidth,
  BankPipelineDepth, StageLatency, PostPipelineBufSize,
  BankSelectorLog2InterleaveSize, RowBufferSizeLog2, RowMissDelay,
  BankAddrConvKind/"interleaving" + BankAddr* fields, Addr* fields,
  Capacity). `WithStorage(s)` → `WithResources(Resources{Storage})`.
- `WithTopPortBufferSize` → external port's PortSpec.BufSize.
- New "Control" port (memcontrolprotocol) must be assigned at assembly.
- `WithBankSelector(custom)` removed (only interleaved).

## amd/driver (ported)

Dependents: samples/runner + timingconfig + emusystem, benchmarks
(gputensor, mccl), server.

- Host-facing API unchanged (Init, AllocateMemory, MemCopyH2D/D2H/D2D,
  LaunchKernel, Run, Terminate, command queues, Distribute, ...).
- `Driver.GPUs []sim.Port` → `[]messaging.RemotePort`;
  `RegisterGPU(p messaging.RemotePort, props)` — pass `port.AsRemote()`.
- Builder: `WithRegistrar(reg)` + `WithSpec(driver.Spec{Freq, Log2PageSize,
  UseMagicMemoryCopy, D2HCycles, H2DCycles})` from `DefaultSpec()` +
  `WithResources(driver.Resources{PageTable, GlobalStorage})`.
- Port: Build declares `driver.GPUPortName` ("GPU"); config builds the port
  externally and `AssignPort(driver.GPUPortName, p)` (v4 auto-created
  40M-deep "ToGPUs" buffers — size is now the config's choice).
- `Command.GetID()` → uint64; `Reqs []messaging.Msg`;
  `CommandHookInfo.Now`/`ReqHookInfo.Now` → `timing.VTimeInPicoSec`;
  `ReqHookInfo.CommandID` → uint64.
- `Build(name)` honors the name (v4 hardcoded "Driver").

## amd/timing/cu (ported)

Dependents: shaderarray config, CP resource pool, runner report/tracers.

- `*cu.ComputeUnit` (component) → `*cu.Comp`; runtime guts via
  `cu.MiddlewareOf(comp) *cu.ComputeUnit`.
- Builder: `WithRegistrar(reg)` + `WithSpec(cu.DefaultSpec())` (fields:
  Freq, SIMDCount, WfPoolSize, VGPRCounts, SGPRCount, LDSBytes,
  Log2CachelineSize, NumSinglePrecisionUnits, VecMemInst/Trans pipeline
  dims, MemPipelineBufferSize, MaxCoalescingPenalty, RegisterScoreboard,
  InFlightVectorMemAccessLimit, InstBufByteSize) +
  `WithResources(cu.Resources{Decoder, ALU, VectorMemModules})`.
- Destinations: `comp.State.InstMem` / `State.ScalarMem` (RemotePort,
  post-build OK); VectorMemModules mapper pre-Build via Resources.
- Ports declared: "Top", "Ctrl", "InstMem", "ScalarMem", "VectorMem" —
  build externally with v4 buffer sizes for parity: Top 4, Ctrl 4,
  InstMem 4, ScalarMem 32, VectorMem 64; AssignPort.
- `WithVisTracer` dropped (simulation auto-attaches); per-SIMD pipeline
  tracing attach externally on `MiddlewareOf(comp).SIMDUnit[i]`.
- Capability methods → `cu.DispatcherView{CU: comp}` adapter.
- `NewISADebugger(logger, cu.MiddlewareOf(comp))`;
  `NewCPIStackInstHook(comp, tt)`; `NewInstTracer(tt timing.TimeTeller)`.
- Behavior delta: pipeline-restart ack waits for Ctrl capacity (v4
  panicked under backpressure).

## amd/timing/cp (ported)

Dependents: timingconfig core + r9nano/mi300a/shaderarray configs.

- `cp.MakeBuilder()`: `WithRegistrar(reg)` + `WithSpec(cp.DefaultSpec())`
  (Freq, NumDispatchers, ConstantKernelLaunchOverhead,
  ConstantKernelOverhead, SubsequentKernelLaunchOverhead,
  WGScalingThreshold). Kept: `WithMonitor(*monitoring2.Monitor)`,
  `WithVisTracer`, `WithCU`, `WithDriver(messaging.RemotePort)`.
  PerfAnalyzer option removed.
- `*cp.CommandProcessor` → `*cp.Comp`. `cp.RegisterCU(comp, cu)` is a
  package function; `CUInterfaceForCP` = DispatchingPort/WfPoolSizes/
  VRegCounts/SRegCount/LDSBytes/ControlPort, all RemotePort-based —
  satisfied by `emu.DispatcherView` and `cu.DispatcherView`.
- Ports (external build + AssignPort): ToDriver, ToDMA, ToCUs, ToTLBs,
  ToAddressTranslators, ToCaches, ToRDMA. Use ~4096 buffers for parity.
- Config must set State RemotePort lists: Driver, DMAEngine, RDMA, CUs,
  TLBs, AddressTranslators, **ROBs (new, split from ATs)**, L1V/L1S/L1I
  Caches, L2Caches, DRAMControllers. Cache/TLB/AT/ROB/DRAM entries are
  the components' "Control" ports.
- DMA: `cp.MakeDMAEngineBuilder().WithRegistrar(reg).WithSpec(
  cp.DefaultDMASpec()).WithResources(cp.DMAResources{LocalDataSource:
  mapper}).Build(name)`; ports "ToCP"/"ToMem" external; `*cp.DMAComp`.
- Behavior deltas: shootdown drains (not discards) in-flight cache
  transactions; L1-then-L2 sequencing; driver commands stall during any
  active control sequence; driver rsps CanSend-queued.

## amd/samples/runner/emusystem (ported)

Dependents: runner core.

- `emusystem.Builder.Build()` returns `*driver.Driver` (was `*sim.Domain`).
  The runner must take the driver from the return value —
  `GetComponentByName("Driver").(*driver.Driver)` no longer works (the
  registered entity is the inner Comp, not the wrapper).
  `report.go`'s `GetComponentByName("Driver").(tracing.NamedHookable)`
  still works.
- `emugpu.Builder.Build(name)` returns `*emugpu.GPU{Name,
  CommandProcessorPort}`; `WithDriver` removed.
- The emu platform uses a minimal in-package command processor (kernel →
  WG decomposition; FlushReq acked immediately); magic memory copy via
  shared storage + page table; one emu CU per GPU (reported CUCount
  stays 64).

## amd/samples/runner + samples + tests (ported)

Runner core:

- `Runner` keeps the driver returned by `emusystem`/`timingconfig`
  `Build()` in a `gpuDriver` field; `Driver()` returns it directly (no
  more `GetComponentByName("Driver").(*driver.Driver)`).
- `Runner.Engine()` returns `timing.Engine`; current time is
  `timing.VTimeInPicoSec`.
- Flags map 1:1 onto the v5 simulation builder: `-parallel` →
  `WithParallelEngine`, `-disable-rtm` → `WithoutMonitoring`,
  `-akitartm-port` → `WithMonitorPort`, `-trace-vis` →
  `WithVisTracingOnStart`. `-metric-file-name` now maps to
  `WithOutputFileName`, but only when explicitly passed, so the default
  output stays `akita_sim_<id>.sqlite3` (the deterministic test script
  depends on that name). `-trace-vis-start/-end`, `-trace-vis-db*`,
  `-buffer-level-trace-*`, `-analyzer-*`, `-trace-mem`, and `-max-inst`
  were already dead in the v4 runner and stay declared-but-unused.
- report.go: `tracing.NewBusyTimeTracer(filter)` /
  `NewAverageTimeTracer(filter)` (no engine arg);
  `StepCountTracer.GetStepCount` → `TagCountTracer.GetTagCount`. Filters
  take `tracing.TaskStart`. Times are picoseconds; reported metrics are
  converted to seconds so units match v4. CU frequency comes from
  `comp.(*cu.Comp).Spec().Freq`.
- Task `What` matching: driver commands keep the pointer type string
  (`*driver.LaunchKernelCommand`); message-derived tasks use the bare
  type name (v5 `msgTypeName`), so `*protocol.LaunchKernelReq` →
  `"LaunchKernelReq"`, `*mem.ReadReq`/`WriteReq` →
  `"ReadReq"`/`"WriteReq"` (Detail is a value `memprotocol.ReadReq`).
- SIMD busy-time tracers attach to `cu.MiddlewareOf(comp).SIMDUnit`
  entries (SIMD units are no longer registered components).
- insttracer/dramtracer embed `tracing.NopTracer`; inflight maps are
  keyed by `uint64`; dram latency derives from stamped
  `TaskStart.Time`/`TaskEnd.Time`.

Platform fixes found during smoke runs:

- **PCIe replaced by a direct connection** (`InterDeviceConn`) in
  timingconfig. The v5.0.0-beta.2 switching network (pcie included) is
  traffic-only: endpoints deliver metadata-only
  `packetization.AssembledMsg` instead of the original message, so no
  payload-carrying protocol can cross it. Until upstream adds payload
  delivery, driver↔GPU/MMU/RDMA traffic uses directconnection (PCIe
  latency is NOT modeled — parity risk).
- **L1I/L1S cache DirLatency 0 → 1**: a v5 `queueing.Pipeline` with 0
  stages never releases items (deadlock). +1 cycle vs v4.
- **L1V TLB Latency 1 → 2**: the v5 TLB middleware inserts requests
  with `AcceptWithDelay(…, 1)`; in a 1-stage pipeline the dwell counter
  of items already at the last stage is never decremented (upstream
  `queueing.Pipeline.advanceItems` caps phase-2 at lastStage-1), so a
  Latency=1 TLB deadlocks. Report upstream.

Tests: deterministic mains return `timing.VTimeInPicoSec`.
`go test ./...`: only pre-existing failures in
`amd/benchmarks/dnn/training{,/optimization}` (mockgen mocks never
committed; v4 upstream fails identically).
