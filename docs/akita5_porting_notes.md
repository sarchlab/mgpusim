# Akita v5 port — cross-package notes

> Historical notes for the earlier Akita port. The Driver wrapper, asynchronous
> host execution, and Runner construction APIs below were replaced by the
> [Runtime and benchmark Step model](runtime-and-benchmark-steps.md). Use that
> guide for current host APIs and checkpoint integration. The AMD HTTP server
> mentioned below has been removed.


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

Dependents: timingconfig builders (r9nano, mi300x), CP ctrlMiddleware.

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

- Instructions are stored by `uint64` ID in CU State.
- `ExecutionState.LastFetchTime` uses `timing.VTimeInPicoSec`; the CU owns the value.
- Sampled completion work belongs to `cu.State.SampledCompletions`, with each
  due time requested once through `comp.TickAt`. The CU has no extra handlers.
- Wavefront and work-group execution views moved to `cu`; `wavefront` retains
  the shared execution-value definitions and generation-bearing local `Handle`. Work-group requests are canonical `messaging.Msg` values in
  CU State. There are no standalone pointer-graph constructors.

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

Dependents: mi300x config (simplebankedmemory), shaderarray config (note:
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

simplebankedmemory (affects mi300x buildDRAMControllers):
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

- `*cu.ComputeUnit` (component) → `*cu.Comp`. All mutable execution data lives
  in `comp.State`; there is no CU Runtime or per-SIMD checkpoint resource.
- Builder: `WithSimulation(simulation)` + `WithSpec(spec)`, starting from
  `cu.DefaultSpec()`. Geometry, frequency, pipeline dimensions, scoreboard
  configuration and memory limits belong to Spec. Shared decoder, ALU,
  sampler and vector-memory mapper belong to `WithResources(cu.Resources{...})`.
- Destinations: `spec.InstMem` and `spec.ScalarMem` are immutable RemotePorts.
  Build the downstream instruction/scalar ROBs and bind their ports first;
  copy their actual `Ports.Top.AsRemote()` addresses into Spec before building
  the CU. `Resources.VectorMemModules` remains the vector-memory mapper.
- Ports: `"Top"`, `"InstMem"`, `"ScalarMem"`, `"VectorMem"`. Build
  them externally with `twowaybuffered.MakeBuilder().WithSimulation(simulation)`
  and bind with `comp.BindPort(name, port)` before connecting. Buffer capacities
  for parity: Top 4, InstMem 4, ScalarMem 32, VectorMem 64. Access bound
  ports through `comp.Ports`.
- Wire the complete topology, then call `simulation.Initialize()`. Only then
  obtain behavior views using `cu.MiddlewareOf(comp) *cu.ComputeUnit`, including
  for `NewISADebugger(logger, cu.MiddlewareOf(comp))`. Wave/group views now live
  in `cu` and carry owner, local slot handle, and global UID; they resolve current
  State on each operation.
  State replacement needs no rebinding or next tick, and initialization is not
  rerun during restore. Fixed `State.Waves` / `State.Groups` arrays own hardware
  slots. Local generations reject recycled handles; global UIDs remain in
  tracing/protocol. Execution ends stop fetch; original slots and CP resources
  remain occupied until dependent work drains and completion can be sent. ALU LDS access
  is scoped to one execution call; retained tracing detail uses IDs and scalar
  metadata rather than wave/instruction objects.
- `WithVisTracer` is dropped. SIMD tasks retain their original owner names and
  labels, but SIMD views are not registered simulation entities. Subscribe one
  simulation observer with `simulation.TaskTracker().Subscribe(observer)` and
  route completed `pipeline` tasks by their SIMD name to per-SIMD tracers.
  Reporter setup derives those names from CU names and `Spec.SIMDCount`, so it
  does not need middleware views before Initialize.
- Capability methods → `cu.DispatcherView{CU: comp}` adapter.
- Other tracing helpers: `NewCPIStackInstHook(comp, tt)` and
  `NewInstTracer(tt timing.TimeTeller)`.
- The legacy CU pipeline flush/restart and shadow-buffer replay API is removed.
  Page migration does not flush or replay CU execution.

See [Compute-unit State and construction](akita-v5-rc2.md#compute-unit-state-and-construction)
for the canonical-state lifecycle and the future Blueprint migration.

## amd/timing/cp (ported)

Dependents: timingconfig core + r9nano/mi300x/shaderarray configs.

- `cp.MakeBuilder().WithSimulation(sim).WithSpec(spec).Build(name)` builds
  `*cp.Comp`. Configure timing and dispatch parameters through `cp.Spec`;
  `WithSampler` and `WithMonitor` supply shared Resources before Build.
- Build DMA, RDMA, compute units and caches first. Set `spec.Driver`,
  `spec.DMAEngine`, `spec.RDMA`, cache control-address slices, and `spec.CUs`
  before CP Build. Populate CU records with
  `cp.DescribeCU(cu.DispatcherView{CU: comp})` in dispatch order.
  `WithDriver`, `WithCU`, and `RegisterCU` are removed; destinations are never
  written to component State. Unused DRAM destinations are not retained.
- Ports are externally built and bound with `BindPort`: ToDriver, ToDMA,
  ToCUs, ToCaches, ToRDMA. Use approximately 4096-entry buffers for parity.
- Dispatcher cursors, in-flight work, rate gates and shared CU reservations
  belong to CP State. Initialize constructs behavior views; dispatchers and
  the pool are no longer separately registered checkpoint resources.
- DMA now lives in `amd/timing/dma`:
  `dma.MakeBuilder().WithSimulation(sim).WithSpec(dma.DefaultSpec()).WithResources(dma.Resources{LocalDataSource: mapper}).Build(name)`
  returns `*dma.Comp`. Bind its external "ToCP" and "ToMem" ports. Routing
  through `LocalDataSource` remains deferred to #321.
- Legacy GPU shootdown/restart commands and their CU/AT/ROB/TLB control
  sequence are removed. Ordinary driver cache flush still drains, flushes,
  invalidates L1 then L2, and enables caches. Kernel-start L1 invalidation and
  independent RDMA drain/restart remain. Driver responses wait for port capacity.

See [DMA package and command processor ownership](akita-v5-rc2.md#dma-package-and-command-processor-ownership)
for current construction and checkpoint compatibility requirements.

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
- SIMD busy-time tracers receive completed pipeline tasks through one global
  task observer, routed by SIMD name. No scoped subscription or pre-Initialize
  `cu.MiddlewareOf` call is needed.
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
