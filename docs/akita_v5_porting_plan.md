# Akita V5 Porting Plan for MGPUSim-dev

> **Status:** Planning only — no code changes.  
> **Date:** 2026-03-19  
> **Reference docs:**  
> - `migration.md` in the upstream Akita repository — V4→V5 migration guide (11 sections)
> - `mgpusim_v5_migration_plan.md` in the upstream Akita repository — high-level 5-phase plan
> - `spec.md` in the upstream Akita repository — V5 component model specification

---

## Table of Contents

1. [Part 1: API Usage Inventory](#part-1-api-usage-inventory)
2. [Part 2: V4→V5 API Mapping Table](#part-2-v4v5-api-mapping-table)
3. [Part 3: Risk Assessment](#part-3-risk-assessment)
4. [Part 4: Phased Migration Plan](#part-4-phased-migration-plan)
5. [Part 5: Preconditions](#part-5-preconditions)

---

## Part 1: API Usage Inventory

### Summary

- **Total Go files in repo (excl. workspace):** 498
- **Files importing `akita/v4`:** 128 (25.7% of codebase)
- **Akita V4 dependency in `go.mod`:** `github.com/sarchlab/akita/v4 v4.9.0`

### Global Import Distribution by Akita V4 Package

| V4 Package | Import Count | V5 Equivalent |
|---|---:|---|
| `akita/v4/sim` | 100 | `akita/v5/sim` |
| `akita/v4/mem/mem` | 42 | `akita/v5/mem` |
| `akita/v4/mem/vm` | 25 | `akita/v5/mem/vm` |
| `akita/v4/tracing` | 24 | `akita/v5/tracing` (or `akita/v5/daisen`) |
| `akita/v4/simulation` | 10 | `akita/v5/simulation` |
| `akita/v4/sim/directconnection` | 9 | `akita/v5/noc/directconnection` |
| `akita/v4/pipelining` | 5 | `akita/v5/queueing` |
| `akita/v4/mem/vm/tlb` | 5 | `akita/v5/mem/vm/tlb` |
| `akita/v4/monitoring` | 3 | `akita/v5/daisen` |
| `akita/v4/mem/vm/mmu` | 3 | `akita/v5/mem/vm/mmu` |
| `akita/v4/mem/idealmemcontroller` | 3 | `akita/v5/mem/idealmemcontroller` |
| `akita/v4/mem/cache` | 3 | `akita/v5/mem/cache` |
| `akita/v4/mem/cache/writeback` | 2 | `akita/v5/mem/cache/writeback` |
| `akita/v4/noc/networking/pcie` | 1 | `akita/v5/noc/networking/pcie` |
| `akita/v4/mem/vm/addresstranslator` | 1 | `akita/v5/mem/vm/addresstranslator` |
| `akita/v4/mem/dram` | 1 | `akita/v5/mem/dram` |
| `akita/v4/mem/cache/writethrough` | 1 | `akita/v5/mem/cache/writethrough` |
| `akita/v4/mem/cache/writearound` | 1 | `akita/v5/mem/cache/writearound` |
| `akita/v4/datarecording` | 1 | TBD (may be removed/merged) |
| `akita/v4/analysis` | 1 | Removed in V5 |

### Per-Subsystem Inventory

---

#### 1. `amd/timing/cu/` — Compute Unit (23 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 17 |
| `akita/v4/mem/mem` | 10 |
| `akita/v4/tracing` | 9 |
| `akita/v4/pipelining` | 3 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded in `ComputeUnit` struct; constructed via `sim.NewTickingComponent()` (1 call)
- **Port creation**: `sim.NewPort()` — 5 calls (ToACE, ToInstMem, ToScalarMem, ToVectorMem, ToCP)
- **Pipelining**: `pipelining.Pipeline` interface (2 instances: `instructionPipeline`, `transactionPipeline`); `pipelining.MakeBuilder()` — 2 calls; `pipelining.PipelineItem` type used in mocks
- **sim.Buffer**: `sim.Buffer` interface — 2 instances (`postInstructionPipelineBuffer`, `postTransactionPipelineBuffer`); `sim.NewBuffer()` — 2 calls
- **Tracing**: `tracing.CollectTrace()`, `tracing.TraceReqInitiate()`, `tracing.TraceReqFinalize()`, `tracing.TraceReqComplete()`, `tracing.BusyTimeTracer`, `tracing.Task` (map[string]tracing.Task in CPIStackTracer and insttracer)
- **VTimeInSec**: `TickScheduler.CurrentTime()` — referenced for timestamps
- **Message handling**: Type-switches on `sim.Msg`, `mem.ReadReq`, `mem.WriteReq`, etc.
- **ID matching**: `.ID == rsp.RespondTo` pattern (4 occurrences for scalar/SIMD/vector memory response matching)
- **Control protocol**: `FlushReq`/`RestartReq` patterns via CU protocol messages
- **Event creation**: `NewEventBase` — 1 call (`wfdispatchevent.go`)

---

#### 2. `amd/timing/cp/` — Command Processor (22 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 20 |
| `akita/v4/tracing` | 6 |
| `akita/v4/mem/mem` | 4 |
| `akita/v4/monitoring` | 3 |
| `akita/v4/mem/cache` | 3 |
| `akita/v4/mem/vm/tlb` | 2 |
| `akita/v4/mem/idealmemcontroller` | 1 |
| `akita/v4/analysis` | 1 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded in DMA engine; `sim.NewTickingComponent()` used
- **Port creation**: `sim.NewPort()` — multiple calls for CP and DMA ports
- **Monitoring**: `monitoring.Monitor` and `monitoring.ProgressBar` used in dispatcher (3 references)
- **Analysis**: `analysis.PerfAnalyzer` used in builder (2 references)
- **ID-keyed maps**: `map[string]*protocol.MapWGReq`, `map[string]*protocol.LaunchKernelReq`, `map[string]*protocol.MemCopyH2DReq`, `map[string]*protocol.MemCopyD2HReq` — 5 maps keyed by string IDs
- **Tracing**: `tracing.TraceReqComplete()` in cpMiddleware
- **Control protocol**: Heavy use of `FlushReq`/`RestartReq` for GPU pipeline flush/restart orchestration; handles `cache.FlushReq`, `tlb` control messages, `idealmemcontroller` control messages
- **Message dispatch**: Extensive type-switch on protocol messages
- **Dispatching subsystem**: `cp/internal/dispatching/` — dispatcher tracks `originalReqs` as `map[string]*protocol.MapWGReq`

---

#### 3. `amd/driver/` — GPU Driver (17 files, incl. 3 in `internal/`)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/mem/vm` | 10 |
| `akita/v4/sim` | 10 |
| `akita/v4/mem/mem` | 4 |
| `akita/v4/tracing` | 1 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded in `Driver` struct
- **Port creation**: `sim.NewPort()` — 2 calls (`Driver.ToGPUs`, `Driver.ToMMU`)
- **VTimeInSec**: Used in tests (`sim.VTimeInSec(11)`, etc.)
- **Tracing**: `tracing.StartTask()`, `tracing.EndTask()`, `tracing.TraceReqInitiate()`, `tracing.TraceReqFinalize()`
- **Memory/VM**: Heavy usage of `mem/vm` package (page table, address translation, PID)
- **Message handling**: Large type-switch in `driver.go` processing FlushReq, GPURestartRsp, RDMARestartRspToDriver, etc.
- **Control protocol**: `protocol.NewFlushReq()`, `protocol.NewGPURestartReq()`, restart/flush orchestration logic
- **ID matching**: `r.Meta().ID == reqID` pattern
- **sim.Msg**: Used extensively for message type assertions

---

#### 4. `amd/samples/runner/` — System Wiring / Platform Builder (11 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 10 |
| `akita/v4/simulation` | 8 |
| `akita/v4/mem/mem` | 8 |
| `akita/v4/sim/directconnection` | 5 |
| `akita/v4/tracing` | 3 |
| `akita/v4/mem/vm/tlb` | 3 |
| `akita/v4/mem/vm/mmu` | 3 |
| `akita/v4/mem/vm` | 3 |
| `akita/v4/mem/idealmemcontroller` | 2 |
| `akita/v4/mem/cache/writeback` | 2 |
| `akita/v4/noc/networking/pcie` | 1 |
| `akita/v4/mem/vm/addresstranslator` | 1 |
| `akita/v4/mem/dram` | 1 |
| `akita/v4/mem/cache/writethrough` | 1 |
| `akita/v4/mem/cache/writearound` | 1 |
| `akita/v4/datarecording` | 1 |

**Key API usage patterns:**
- **Simulation orchestration**: `simulation.MakeBuilder()`, `simulation.Simulation`, `simulation.RegisterComponent()`, `simulation.GetEngine()`, `simulation.GetComponentByName()`, `simulation.Terminate()`
- **DirectConnection**: `directconnection.MakeBuilder()` — 10+ calls across shaderarray, r9nano, and mi300a builders; `directconnection.Comp` type stored as struct fields
- **Cache/memory builders**: `writeback`, `writearound`, `writethrough`, `idealmemcontroller`, `dram`, `tlb`, `mmu`, `addresstranslator` — all V4 builder patterns
- **PCIe**: `noc/networking/pcie` — 1 import for inter-GPU networking
- **Tracing**: `tracing.Tracer` passed to sub-builders
- **DataRecording**: `datarecording.DataRecorder` used in report.go
- **GPU configs**: r9nano builder (1 file), mi300a builder (1 file), shaderarray builder (1 file)

---

#### 5. `amd/timing/mem/` — Memory Timing Models (4 files)

Includes `addresstranslator/` and `simplebankedmemory/`.

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 4 |
| `akita/v4/mem/mem` | 4 |
| `akita/v4/tracing` | 2 |
| `akita/v4/pipelining` | 2 |
| `akita/v4/mem/vm` | 1 |

**Key API usage patterns:**
- **Pipelining**: `pipelining.Pipeline` and `pipelining.MakeBuilder()` in `simplebankedmemory`
- **sim.Buffer**: `sim.NewBuffer()` — 1 call (`postPipelineBuf`)
- **Address translator**: Transaction tracking with `ID` matching — `t.translationReq.ID == id`, `r.reqToBottom.Meta().ID == id`
- **Tracing**: `tracing.TraceReqInitiate()`, `tracing.TraceReqFinalize()`

---

#### 6. `amd/protocol/` — Protocol Messages (2 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 2 |
| `akita/v4/mem/vm` | 2 |

**Key API usage patterns:**
- **MsgMeta**: `sim.MsgMeta` embedded in all protocol message types
- **ID generation**: `sim.GetIDGenerator().Generate()` — 29 calls in driverprotocol.go, 12 calls in cuprotocol.go
- **sim.Msg interface**: All messages implement `sim.Msg` (Meta() and Clone())
- **Message types defined**: `FlushReq`, `LaunchKernelReq`, `MemCopyH2DReq`, `MemCopyD2HReq`, `GPURestartReq`, `GPURestartRsp`, `RDMARestartReqToCP`, `RDMARestartRspToDriver`, `ShootDownCommand`, `ShootDownCompleteRsp`
- **CU protocol**: `cuprotocol.go` defines `MapWGReq`, `WGCompletionMsg`, `CUPipelineFlushReq`, `CUPipelineRestartReq`

---

#### 7. `amd/emu/` — Emulation Engine (11 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/mem/vm` | 8 |
| `akita/v4/mem/mem` | 4 |
| `akita/v4/sim` | 3 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded in emulation CU
- **Port creation**: `sim.NewPort()` — 1 call (`ToDispatcher`)
- **VTimeInSec**: `sim.VTimeInSec(math.Ceil(float64(now)))` — float64 cast for time arithmetic
- **Event creation**: `NewEventBase` referenced for WG completion events
- **VM**: Heavy usage of `mem/vm` — page table, address translation, PID, virtual memory operations
- **Memory access**: `mem.ReadReq`, `mem.WriteReq` for emulated memory operations

---

#### 8. `amd/timing/rdma/` — RDMA Engine (4 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 4 |
| `akita/v4/mem/mem` | 3 |
| `akita/v4/tracing` | 1 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded, `sim.NewTickingComponent()` call, direct `Freq` field assignment (`c.TickingComponent.Freq = freq`)
- **Port creation**: `sim.NewPort()` — 5 calls (RDMARequestInside, RDMARequestOutside, RDMADataInside, RDMADataOutside, CtrlPort)
- **ID matching**: `trans.toOutside.Meta().ID == rspTo`, `trans.toInside.Meta().ID == rspTo`
- **Tracing**: `tracing.TraceReqFinalize()`, `tracing.TraceReqComplete()`
- **Protocol**: Custom RDMA protocol messages (`rdmaprotocol.go`)
- **Control**: Flush/restart handling for RDMA transactions
- **Message dispatching**: Type-switch on incoming messages

---

#### 9. `amd/timing/pagemigrationcontroller/` — Page Migration Controller (3 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 3 |
| `akita/v4/mem/mem` | 2 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded; `sim.NewTickingComponent(name, engine, 1*sim.GHz, e)` — uses `sim.GHz` constant
- **Port creation**: `sim.NewPort()` — 3 calls (RemotePort, LocalMemPort, CtrlPort)
- **ID-keyed maps**: `map[string]uint64` for `reqIDToWriteAddressMap`
- **VTimeInSec**: `TickScheduler.CurrentTime()` for transfer timing
- **Custom protocol**: `pmcprotocol.go` defines PMC-specific messages

---

#### 10. `amd/timing/rob/` — Reorder Buffer (5 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 4 |
| `akita/v4/tracing` | 2 |
| `akita/v4/mem/mem` | 2 |

**Key API usage patterns:**
- **TickingComponent**: `sim.TickingComponent` embedded; `sim.NewTickingComponent()` call
- **Port creation**: `sim.NewPort()` — 3 calls (topPort, bottomPort, controlPort)
- **ID-keyed maps**: `map[string]*list.Element` for `toBottomReqIDToTransactionTable`
- **Tracing**: `tracing.TraceReqReceive()`, `tracing.TraceReqInitiate()`, `tracing.TraceReqFinalize()`, `tracing.TraceReqComplete()`
- **Control protocol**: Handles flush/restart through control port
- **Port hooks**: Custom `porthook.go` for port event tracking

---

#### 11. `amd/timing/wavefront/` — Wavefront Data Structures (3 files)

**Imported V4 packages:**
| Package | Count |
|---|---:|
| `akita/v4/sim` | 3 |
| `akita/v4/mem/vm` | 1 |

**Key API usage patterns:**
- **VTimeInSec**: `sim.VTimeInSec` type used for wavefront timing fields (LastFetchTime, CompletionTime, etc.)
- **Event creation**: `sim.EventBase` embedded in `WfCompletionEvent`
- **sim.Handler**: `sim.Handler` interface referenced

---

#### 12. `amd/kernels/` (1 file)

- Imports `akita/v4/sim` — uses `sim.Component` interface for kernel grid/work-group mapping.

#### 13. `amd/sampling/` (2 files)

- Imports `akita/v4/sim` — `VTimeInSec` used in sampling arithmetic (`stableengine.go` does float-cast: `sim.VTimeInSec(se.granularity)*se.issueTimeSquareSum`).

#### 14. `amd/benchmarks/` (4 files)

- `mccl/` and `dnn/gputensor/` import `akita/v4/simulation` (2) and `akita/v4/sim` (1), `akita/v4/mem/mem` (1).
- Primarily use simulation orchestration APIs.

#### 15. `amd/tests/deterministic/` (2 files)

- Import `akita/v4/sim` — simple test harnesses.

#### 16. `nvidia/` — NVIDIA GPU Models (14 files)

**Imported V4 packages (per sub-package):**
| Sub-package | Files | Packages |
|---|---:|---|
| `nvidia/driver` | 2 | `sim` (2), `sim/directconnection` (2) |
| `nvidia/gpu` | 2 | `sim` (2), `sim/directconnection` (1) |
| `nvidia/sm` | 2 | `sim` (2), `sim/directconnection` (1) |
| `nvidia/subcore` | 2 | `sim` (2) |
| `nvidia/message` | 1 | `sim` (1) |
| `nvidia/platform` | 3 | `sim` (3) |
| `nvidia/runner` | 1 | `sim` (1) |
| `nvidia/nvidia.go` | 1 | — |

**Key API usage patterns:**
- **sim.Port**: Used for component port declarations
- **DirectConnection**: `directconnection.MakeBuilder()` for internal wiring
- **sim.Component**: Component interface usage
- **sim.Msg / MsgMeta**: Message types for NVIDIA protocol
- **sim.NewPort()**: Port creation pattern (estimated 10+ calls)
- **TickingComponent**: Used across driver, gpu, sm, subcore

#### 17. Root-level Files

- **`go.mod`**: `github.com/sarchlab/akita/v4 v4.9.0` — single primary dependency line; commented-out `replace` directive for local dev
- **`go.sum`**: Contains akita/v4 hash

---

### API Pattern Summary Across All Subsystems

| API Pattern | Occurrences | Files Affected |
|---|---:|---:|
| `sim.Port` references | 99 | ~40 |
| `sim.NewPort()` calls | 48 | ~20 |
| `sim.Msg` usage | 122 | ~35 |
| `MsgMeta` references | 107 | ~30 |
| `VTimeInSec` usage | 25 files | 25 |
| `sim.Freq` / frequency | 19 files | 19 |
| `sim.TickingComponent` | 14 refs | ~10 |
| `sim.Engine` references | 36 | ~15 |
| `simulation.*` calls | 136 | ~11 |
| `tracing.*` calls | 237 | ~20 |
| `FlushReq/FlushRsp` refs | 130 | ~15 |
| `RestartReq/RestartRsp` refs | 167 | ~15 |
| `DrainReq/DrainRsp` refs | 67 | ~8 |
| `IDGenerator` refs | 77 | ~15 |
| `RspTo` references | 112 | ~20 |
| `map[string]` (ID-keyed) | 55 | ~15 |
| `pipelining.*` refs | 17 | ~5 |
| `monitoring.*` refs | 10 | ~4 |
| `directconnection.*` refs | 31 | ~6 |
| `sim.Buffer` / `NewBuffer` | 7 | 5 |
| Hook-related refs | 77 | ~15 |
| `.Send()` calls | 103 | 28 |
| `NewEventBase` calls | 4 | 4 |

---

## Part 2: V4→V5 API Mapping Table

Each row maps a V4 pattern found in the codebase to its V5 equivalent, with a reference to the corresponding section in `migration.md`.

### 2.1 Time and Frequency (migration.md §1)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `type VTimeInSec float64` (seconds) | `type VTimeInSec uint64` (picoseconds) | Same type name, different unit and underlying type |
| `sim.VTimeInSec(0.000000001)` (1 ns) | `sim.VTimeInSec(1000)` (1000 ps) | All literal time values must be converted |
| `sim.Freq(1e9)` | `1 * sim.GHz` | Use predefined constants: `Hz`, `KHz`, `MHz`, `GHz` |
| `1.0 / float64(freq)` | `freq.Period()` | Returns picoseconds per cycle |
| `now + period` | `freq.NextTick(now)` | Next tick boundary after now |
| `math.Ceil(float64(now))` (in emu) | `freq.ThisTick(now)` | Ceil to nearest tick boundary |
| `float64` time arithmetic (`+`, `-`, `*`, `/`) | `uint64` arithmetic or `Freq` helpers | **25 files affected** |
| `time == 0.0` checks | `time == 0` | |
| `VTimeInSec` in sampling arithmetic | Rewrite with uint64 math | `stableengine.go` does float casts |
| `1*sim.GHz` (already used in PMC) | Same — already correct syntax | PMC already uses `1*sim.GHz` |

### 2.2 Entity IDs (migration.md §2)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `MsgMeta.ID string` | `MsgMeta.ID uint64` | |
| `MsgMeta.RspTo string` | `MsgMeta.RspTo uint64` | |
| `sim.GetIDGenerator().Generate()` → `string` | `sim.GetIDGenerator().Generate()` → `uint64` | 29 calls in driverprotocol.go, 12 in cuprotocol.go |
| `map[string]*SomeReq` (ID-keyed) | `map[uint64]*SomeReq` | ~15 maps across CP, dispatcher, ROB, PMC, RDMA |
| `ID == ""` / `!= ""` sentinel | `ID == 0` / `!= 0` | |
| `tracing.Task.ID string` | `tracing.Task.ID uint64` | CPIStackTracer `map[string]tracing.Task` → `map[uint64]tracing.Task` |
| `req.ID + "_fetch"` string concat | Separate ID generation | scheduler.go constructs compound IDs |
| `fmt.Sprintf`-based ID formatting | `strconv.FormatUint` or `%d` | |

### 2.3 Unified Control Protocol (migration.md §3)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `FlushReq` / `FlushRsp` | `ControlReq{Command: mem.CmdFlush}` / `ControlRsp` | 130 references |
| `RestartReq` / `RestartRsp` | `ControlReq{Command: mem.CmdEnable}` or `CmdReset` / `ControlRsp` | 167 references |
| `DrainReq` / `DrainRsp` | `ControlReq{Command: mem.CmdDrain}` / `ControlRsp` | 67 references |
| `cache.FlushReq` (from CP) | `mem.ControlReq{Command: mem.CmdFlush}` | CP dispatches flush to caches, TLBs, mem controllers |
| `protocol.FlushReq` (driver→GPU) | `mem.ControlReq{Command: mem.CmdFlush}` | Driver-level flush command |
| `protocol.GPURestartReq` / `GPURestartRsp` | `mem.ControlReq{Command: mem.CmdEnable}` / `ControlRsp` | GPU restart after flush |
| `protocol.RDMARestartReqToCP` | `mem.ControlReq{Command: mem.CmdEnable}` | RDMA restart |
| `CUPipelineFlushReq` / `CUPipelineRestartReq` | Unified `ControlReq` with appropriate command | CU pipeline control |
| Separate type-switch cases per control msg | Single `ControlRsp` type-switch on `.Command` | Simplifies handler code |
| `DiscardInflight`, `InvalidateAfter`, `PauseAfter` flags | New V5 `ControlReq` fields | More fine-grained control |

### 2.4 Event Serialization (migration.md §4)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `event.Handler() Handler` | `event.HandlerID() string` | String-based handler lookup |
| `sim.NewEventBase(time, handlerObj)` | `sim.NewEventBase(time, "handlerName")` | 4 calls in codebase |
| Handler as interface pointer | Handler registered via `engine.RegisterHandler(name, handler)` | Auto-registered by `NewTickingComponent` |
| `WfCompletionEvent` with embedded `EventBase` | Update to use `HandlerID_` string | wavefront/wfcompletionevent.go |
| `WGCompleteEvent` (emu) | Update handler reference | emu/wgcompleteevent.go |

### 2.5 In-Place State Update (migration.md §5)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| Double-buffered state (deep copy of `current` → `next`) | In-place update (`current` and `next` are shallow copies of the same struct) | Paradigm shift — V5 `GetState()` and `GetNextState()` return the same data during a tick |
| `state := comp.GetState()` (snapshot from start of tick) | `state := comp.GetState()` (live data, same as next) | Read from either; they are identical during Tick |
| `next := comp.GetNextState()` (separate copy) | `next := comp.GetNextState()` (pointer to same data as current) | Mutations are immediately visible through `GetState()` |
| `comp.CommitNextState()` (deep copy next → current) | No explicit commit needed — `Tick()` handles it | Remove all `CommitNextState()` calls |
| Deep-copy logic between state snapshots | Remove — no longer needed | Any custom clone/copy code for state is dead code in V5 |
| **Impact on mgpusim-dev:** V4 patterns (`GetState`/`GetNextState`/`CommitNextState`) are **not currently used** in mgpusim-dev. Components use direct struct fields for state. The migration to `modeling.Component[S,T]` (§6 below) will introduce these patterns for the first time. | | Risk is in the §6 component model migration, not here directly. |

### 2.6 Component Model (migration.md §6)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `sim.TickingComponent` struct | `sim.TickingComponent` (updated) + `modeling.Component[S,T]` | V5 uses generic component with Spec+State |
| `sim.NewTickingComponent(name, engine, freq, ticker)` | `modeling.NewBuilder[S,T]().WithEngine(e).WithFreq(f).Build(name)` | Builder pattern |
| `sim.ComponentBase` | `sim.NewComponentBase(name)` | |
| Direct struct fields for state | State struct `T` accessed via `GetState()`/`GetNextState()` | In-place state update model |
| Manual Tick logic | Middleware pipeline: `AddMiddleware(&mw{})` | Per-tick behavior as middleware chain |
| Internal port construction | External port creation + `AddPort("name", port)` | Ports injected during wiring |
| `port.SetComponent(comp)` | Same — available in V5 | Decoupled port/component lifecycle |

### 2.7 Port Creation (migration.md §8)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `sim.NewPort(component, inBuf, outBuf, name)` | `sim.NewPort(nil, inBuf, outBuf, name)` then `port.SetComponent(comp)` | 48 calls to update |
| Ports created inside component builder | Ports created externally, injected via `WithXxxPort()` | Architectural change for all builders |
| `sim.Port` interface | `sim.Port` interface (updated) | `SetComponent()` added |

### 2.8 Pipelining → Queueing (migration.md §9)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `pipelining.Pipeline` interface | `queueing.Pipeline[T]` struct literal | 4 instances in CU + simplebankedmemory |
| `pipelining.MakeBuilder().WithNumStage(n).Build()` | `queueing.Pipeline[T]{NumStages: n, Width: 1}` | 3 builder calls |
| `pipelining.PipelineItem` interface | Generic type parameter `T` | |
| `sim.Buffer` interface | `queueing.Buffer[T]` struct literal | 3 instances |
| `sim.NewBuffer(name, cap)` | `queueing.Buffer[T]{BufferName: name, Cap: cap}` | 3 calls |

### 2.9 Monitoring → Daisen (migration.md, spec.md)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `monitoring.Monitor` | `daisen` package equivalent | 3 references in CP dispatcher |
| `monitoring.ProgressBar` | `daisen` equivalent or removed | 1 reference |
| `datarecording.DataRecorder` | TBD (may be merged into daisen) | 1 reference in report.go |
| `analysis.PerfAnalyzer` | Removed in V5 | 2 references in CP builder |

### 2.10 Tracing (migration.md §2, spec.md)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `tracing.Tracer` interface | `tracing.Tracer` (mostly compatible) | |
| `tracing.Task.ID string` | `tracing.Task.ID uint64` | |
| `tracing.Task.ParentID string` | `tracing.Task.ParentID uint64` | |
| `tracing.StartTask()` / `tracing.EndTask()` | Same function names (updated types) | |
| `tracing.TraceReqInitiate()` / `TraceReqFinalize()` / `TraceReqComplete()` | Same functions (IDs become uint64) | |
| `tracing.CollectTrace()` | Same or updated | |
| `tracing.BusyTimeTracer` | Same or updated | |
| `map[string]tracing.Task` | `map[uint64]tracing.Task` | CPIStackTracer, insttracer |
| `MsgMeta.SendTaskID` / `RecvTaskID` | New fields in V5 `MsgMeta` | |

### 2.11 DirectConnection Path Change

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `akita/v4/sim/directconnection` | `akita/v5/noc/directconnection` | 9 imports, 31 references |
| `directconnection.MakeBuilder()` | Same API, new import path | |
| `directconnection.Comp` type | Same type, new import path | |

### 2.12 Simulation Package

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `simulation.MakeBuilder()` | `simulation.MakeBuilder()` (updated) | 136 references across runner |
| `simulation.Simulation` | `simulation.Simulation` | |
| `simulation.RegisterComponent()` | `simulation.RegisterComponent()` | |
| `simulation.GetEngine()` | `simulation.GetEngine()` | |
| `simulation.GetComponentByName()` | `simulation.GetComponentByName()` | |

### 2.13 DRAM Improvements (migration.md §7)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `idealmemcontroller.MakeBuilder().WithLatency(n).Build()` | `dram.MakeBuilder().WithSpec(dram.HBM2Spec).Build()` | Cycle-accurate DRAM replaces fixed-latency controller |
| `idealmemcontroller.Comp` type | `dram.Comp` with bank-state machine | Bank-level parallelism, command sequencing |
| `akita/v4/mem/idealmemcontroller` import (3 files) | `akita/v5/mem/dram` | r9nano/builder.go, emugpu/builder.go, commandprocessor.go |
| `akita/v4/mem/dram` import (1 file: r9nano) | `akita/v5/mem/dram` (same, API updated) | r9nano already imports dram but uses idealmemcontroller |
| Fixed-latency memory model | Configurable presets: `DDR4Spec`, `HBM2Spec`, `HBM3Spec`, `GDDR6Spec` | **Optional:** can still use idealmemcontroller if V5 retains it; otherwise must migrate to `dram` |

### 2.14 CLI Changes (migration.md §10)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `akita check [path]` | `akita component-lint [path]` | Command renamed; same functionality |
| `akita check ./...` | `akita component-lint ./...` | Recursive check; dirs without `//akita:component` reported as `not a component` (not a failure) |
| `component --create <path>` | `akita component-create <path>` | New scaffolding command |
| **Impact on mgpusim-dev:** No Go code changes needed — CLI commands appear only in CI workflows and developer documentation. | | Update any Makefile or workflow references to `akita check`. |

### 2.15 CI Migration (migration.md §11)

| V4 Pattern | V5 Equivalent | Notes |
|---|---|---|
| `runs-on: ubuntu-latest` (GitHub-hosted) | `runs-on: self-hosted` | Akita V5 CI uses self-hosted runners |
| **mgpusim-dev status:** Already uses `runs-on: self-hosted` in all workflows | No change needed | All 4 workflow files already on self-hosted runners |
| `go generate ./...` from repo root | `go generate ./...` from `v5/` directory | Only relevant when running akita's own tests |
| Workflow trigger: `push` only | Trigger on both `push` and `pull_request` | Align mgpusim-dev workflows if needed |

---

## Part 3: Risk Assessment

### Risk Summary Table

| Subsystem | Files | Risk | Justification |
|---|---:|:---:|---|
| `amd/timing/cu/` | 23 | **HIGH** | Pipelining→queueing rewrite, extensive tracing, ID matching, control protocol, CPI stack tracer refactor |
| `amd/timing/cp/` | 22 | **HIGH** | Control protocol orchestration (flush/restart/drain), monitoring→daisen, analysis removal, 5+ ID-keyed maps, dispatcher refactor |
| `amd/driver/` | 17 | **HIGH** | Control flow orchestration (flush→restart→RDMA restart), tracing, VTimeInSec in tests, many message type-switches |
| `amd/samples/runner/` | 11 | **HIGH** | Widest import surface (16 unique V4 packages), all builder APIs change, directconnection path, simulation wiring, config for r9nano+mi300a |
| `amd/timing/mem/` | 4 | **MEDIUM** | Pipelining→queueing, sim.Buffer, ID matching in address translator |
| `amd/protocol/` | 2 | **HIGH** | All message types need MsgMeta update (string→uint64 IDs), 20 IDGenerator calls, FlushReq/RestartReq types removed |
| `amd/emu/` | 11 | **MEDIUM** | TickingComponent update, VTimeInSec float cast, event creation, VM package usage (mostly path changes) |
| `amd/timing/rdma/` | 4 | **MEDIUM** | TickingComponent, port creation, ID matching, control protocol, Freq field assignment |
| `amd/timing/pagemigrationcontroller/` | 3 | **MEDIUM** | TickingComponent, port creation, ID-keyed map, VTimeInSec timing |
| `amd/timing/rob/` | 5 | **MEDIUM** | TickingComponent, port creation, ID-keyed map, tracing, control port |
| `amd/timing/wavefront/` | 3 | **LOW** | VTimeInSec type change, EventBase update — mostly mechanical |
| `amd/kernels/` | 1 | **LOW** | Only sim.Component interface usage |
| `amd/sampling/` | 2 | **MEDIUM** | VTimeInSec arithmetic with float casts — requires careful rewrite |
| `amd/benchmarks/` | 4 | **LOW** | Simulation orchestration API only |
| `amd/tests/` | 2 | **LOW** | Simple test harnesses, sim import only |
| `nvidia/` | 14 | **MEDIUM** | TickingComponent, port creation, directconnection, message types — parallel but independent from AMD subsystem |
| Root (`go.mod`) | 1 | **LOW** | Module path update — mechanical |

### Risk Category Definitions

- **LOW (4 subsystems, 10 files):** Only import path changes and mechanical type renames. No logic changes needed. Can be done with find-and-replace.
- **MEDIUM (7 subsystems, 43 files):** Logic changes required — time arithmetic conversion, ID type migration, moderate refactoring of data structures. Testable in isolation.
- **HIGH (5 subsystems, 75 files):** Architectural changes required — control protocol rewrite, pipeline→queue conversion, component model update, monitoring stack replacement, message type redesign.

### Critical Risk Areas

1. **Control Protocol Unification (364 references):** The driver→CP→CU flush/restart/drain flow spans 3 subsystems and uses separate message types that must be unified into `ControlReq`/`ControlRsp`. This is the single highest-risk change due to cross-subsystem coordination.

2. **ID Type Migration (map[string]→map[uint64]):** At least 15 maps keyed by string IDs must change to uint64. Most are straightforward, but the `CPI stack tracer` and `dispatcher` have complex ID-matching logic.

3. **Pipeline/Buffer Conversion:** Only 7 instances, but they're in performance-critical paths (CU vector memory unit, banked memory). The generic `queueing.Pipeline[T]` and `queueing.Buffer[T]` types have different APIs.

4. **Protocol Message Redesign:** `amd/protocol/` defines all inter-component messages. Changing `MsgMeta` from string to uint64 IDs affects every message constructor and every handler that matches on message IDs.

---

## Part 4: Phased Migration Plan

Based on the existing 5-phase plan from `mgpusim_v5_migration_plan.md`, customized for mgpusim-dev specifics.

### Phase 0: Preconditions (Week 0 — parallel prep)

**Goal:** Clear blockers before migration begins.

| Task | Details |
|---|---|
| Remove gfx803/R9Nano config | Complete issue #449; removes `r9nano/builder.go` (1 file, ~15 gfx803 references) |
| Akita V5 beta release | Ensure `akita/v5` module is tagged and consumable |
| Set up migration branch | `akita-v5-migration` long-lived branch |
| `go.mod` dependency update | Update the Akita module requirement to the published V5 module version; avoid committed local-path overrides |
| CI configuration | Ensure CI can build against V5; add V5-specific test matrix |

**Dependencies:** None — can start immediately.  
**Estimated effort:** 0–1 week.

---

### Phase 1: Mechanical Foundation (Weeks 1–3)

**Goal:** Update all import paths and resolve compilation errors from path changes. No logic changes.

**Subsystems in scope:**

| Subsystem | Files | Work |
|---|---:|---|
| All 128 files | 128 | Import path `akita/v4/X` → `akita/v5/X` |
| `go.mod` | 1 | Module dependency update |
| `directconnection` imports | 6 files | `v4/sim/directconnection` → `v5/noc/directconnection` |
| `pipelining` imports | 5 files | `v4/pipelining` → `v5/queueing` |
| `monitoring` imports | 4 files | `v4/monitoring` → `v5/daisen` |
| `analysis` import | 1 file | Remove (package deleted in V5) |
| `datarecording` import | 1 file | Update or remove |

**Approach:**
1. `sed -i` to rename all import paths mechanically.
2. Fix any immediate compilation errors from renamed types/packages.
3. Do NOT fix logic errors yet — just get imports compiling.

**Testing:** `go build ./...` must pass. Tests may fail (expected).  
**Rollback:** Revert import path changes.  
**Estimated effort:** 2–3 weeks (128 files, mostly mechanical).

---

### Phase 2: Type and Message Core (Weeks 3–7)

**Goal:** Migrate time types, ID types, and MsgMeta across the codebase.

**Subsystems in scope (ordered by dependency):**

| Order | Subsystem | Files | Work |
|---:|---|---:|---|
| 1 | `amd/protocol/` | 2 | MsgMeta ID string→uint64, all 20 IDGenerator calls, message Clone() |
| 2 | `amd/timing/wavefront/` | 3 | VTimeInSec type, EventBase |
| 3 | `amd/kernels/` | 1 | sim.Component interface update |
| 4 | `amd/emu/` | 11 | VTimeInSec float→uint64, EventBase, VM types |
| 5 | `amd/sampling/` | 2 | VTimeInSec arithmetic rewrite |
| 6 | `amd/driver/` (types only) | 17 | VTimeInSec in tests, ID matching |
| 7 | `amd/timing/*` (types only) | 39 | ID-keyed maps (string→uint64), VTimeInSec fields |
| 8 | `nvidia/` (types only) | 14 | Message types, VTimeInSec |

**Key changes:**
- `VTimeInSec` float64 → uint64 (25 files)
- `Freq` float64 → uint64 Hz (19 files)
- `MsgMeta.ID` / `RspTo` string → uint64 (107+ references)
- `map[string]*X` → `map[uint64]*X` for ID-keyed maps (15 maps)
- `tracing.Task.ID` string → uint64 (affects CPIStackTracer, insttracer)
- `GetIDGenerator().Generate()` return type changes automatically

**Testing:** Unit tests for protocol, emu, wavefront. Integration tests for ID matching across subsystems.  
**Rollback:** Revert type changes per-subsystem.  
**Estimated effort:** 4–5 weeks.

---

### Phase 3: Protocol and Dataflow (Weeks 7–11)

**Goal:** Rewrite control protocol, convert pipelines/buffers, update event model.

**Subsystems in scope:**

| Order | Subsystem | Files | Work |
|---:|---|---:|---|
| 1 | `amd/protocol/` | 2 | Remove FlushReq/RestartReq types, define ControlReq/ControlRsp usage |
| 2 | `amd/timing/cu/` | 23 | Pipeline→queueing (2 pipelines, 2 buffers), CUPipelineFlushReq→ControlReq, event handler changes |
| 3 | `amd/timing/mem/` | 4 | Pipeline→queueing (1 pipeline, 1 buffer) |
| 4 | `amd/timing/cp/` | 22 | Control protocol rewrite (flush/restart/drain orchestration), monitoring→daisen |
| 5 | `amd/driver/` | 17 | FlushReq→ControlReq, GPURestartReq→ControlReq, handler unification |
| 6 | `amd/timing/rdma/` | 4 | Control protocol, Freq assignment |
| 7 | `amd/timing/rob/` | 5 | Control port handling |
| 8 | `amd/timing/pagemigrationcontroller/` | 3 | Control port handling |

**Key changes:**
- **Control protocol (364 refs):** Replace `FlushReq`/`FlushRsp` (130 refs), `RestartReq`/`RestartRsp` (167 refs), `DrainReq`/`DrainRsp` (67 refs) with unified `ControlReq`/`ControlRsp`
- **Pipeline conversion (17 refs):** `pipelining.Pipeline` → `queueing.Pipeline[T]` (4 instances)
- **Buffer conversion (7 refs):** `sim.Buffer` → `queueing.Buffer[T]` (3 instances)
- **Event model (4 refs):** `NewEventBase(time, handler)` → `NewEventBase(time, "handlerName")`, handler registration
- **Monitoring removal (10 refs):** `monitoring.Monitor` → `daisen` equivalent
- **Analysis removal (2 refs):** `analysis.PerfAnalyzer` — remove or replace

**Critical path:** Driver → CP → CU flush/restart flow must be migrated together to maintain coherence.

**Testing:** Integration tests for flush/restart/drain flow end-to-end. Unit tests for each pipeline/buffer conversion.  
**Rollback:** Revert per-subsystem, but control protocol changes are cross-cutting.  
**Estimated effort:** 3–5 weeks.

---

### Phase 4: Integration Wiring (Weeks 11–15)

**Goal:** Update component model, cache/system builders, and daisen integration.

**Subsystems in scope:**

| Order | Subsystem | Files | Work |
|---:|---|---:|---|
| 1 | All components | ~40 | TickingComponent → modeling.Component[S,T] pattern (optional; can defer) |
| 2 | `amd/samples/runner/` | 11 | Cache builder APIs, directconnection wiring, simulation orchestration |
| 3 | `nvidia/` | 14 | Platform/GPU/SM builders, directconnection |
| 4 | Port creation | ~20 | External port creation pattern (WithXxxPort) |

**Key changes:**
- **Port creation pattern:** `sim.NewPort(component, ...)` → `sim.NewPort(nil, ...)` + `port.SetComponent(comp)` (48 calls)
- **Builder updates:** All component builders adopt V5 `With*Port()` injection pattern
- **Simulation wiring:** `simulation.MakeBuilder()` and `RegisterComponent()` calls updated
- **DirectConnection:** Import path already updated in Phase 1; verify API compatibility
- **Cache builders:** `writeback`, `writearound`, `writethrough` builder APIs may change — depends on V5 cache package
- **DRAM builder:** May switch to V5 cycle-accurate DRAM if desired (optional enhancement)

**Testing:** Full system integration tests — build complete GPU system, run sample benchmarks.  
**Rollback:** Revert builder changes per-config.  
**Estimated effort:** 3–5 weeks.

---

### Phase 5: Validation and Parity (Weeks 15–17)

**Goal:** Ensure correctness and performance parity with V4.

| Task | Details |
|---|---|
| Test repair | Fix all broken unit tests (~50+ test files reference V4 APIs) |
| Integration tests | Run full `amd/tests/deterministic/` suite |
| Benchmark comparison | Run sample workloads, compare cycle counts and wall-clock time |
| Performance profiling | Ensure no regressions from type changes (uint64 vs float64 time) |
| Tracing validation | Verify daisen tracing output matches V4 monitoring output |
| Mock regeneration | `go generate ./...` for all mock files (test files with mock_* prefix) |
| CI green | All CI checks passing on migration branch |

**Estimated effort:** 2–3 weeks.

---

### Phase Summary

| Phase | Weeks | Files | Risk | Dependencies |
|---|---:|---:|:---:|---|
| P0: Preconditions | 0–1 | 2 | Blocker | None |
| P1: Mechanical Foundation | 1–3 | 128 | Low | P0 |
| P2: Type & Message Core | 3–7 | ~80 | Medium | P1 |
| P3: Protocol & Dataflow | 7–11 | ~80 | High | P2 |
| P4: Integration Wiring | 11–15 | ~65 | Medium | P3 |
| P5: Validation & Parity | 15–17 | All | Medium | P4 |

**Total estimated effort:** 14–22 person-weeks.

---

## Part 5: Preconditions

### 5.1 gfx803/R9Nano Removal (Issue #449) — SHOULD complete first

**Why:** The `r9nano/builder.go` file is one of the most complex GPU configuration files (imports 10+ V4 packages, contains directconnection wiring, cache builder chains, memory hierarchy setup). Removing it before migration:
- Reduces the migration surface by 1 full GPU config + ~15 gfx803 references
- Eliminates a legacy architecture that won't be maintained post-V5
- Simplifies testing — one fewer GPU config to validate
- Removes a potential source of merge conflicts during migration

**Current state:** r9nano builder exists at `amd/samples/runner/timingconfig/r9nano/builder.go` (1 file). References to gfx803/R9Nano exist in ~15 locations.

### 5.2 Akita V5 Beta Release

The V5 module must be consumable as a Go module dependency. Currently the upstream Akita development repository has V5 code under a `v5/` subdirectory. Before mgpusim-dev can migrate:
- V5 code must be at the repository root (or on a `v5` branch)
- A beta tag must be published (e.g., `v5.0.0-beta.1`)
- The module path must be `github.com/sarchlab/akita/v5`

**Local-only alternative:** During development, an engineer may temporarily use a private Go
module override that points to their own Akita V5 checkout. Keep local-path overrides out of
committed changes and prefer a tagged beta release for shared branches.

### 5.3 Go Module Dependency Changes

**Current `go.mod`:**
```
github.com/sarchlab/akita/v4 v4.9.0
```

**Target `go.mod`:**
```
github.com/sarchlab/akita/v5 v5.0.0-beta.1
```

All transitive dependencies pulled in by akita/v5 must be compatible. The V5 module may introduce new dependencies (e.g., for daisen integration).

### 5.4 CI / Test Infrastructure

| Requirement | Details |
|---|---|
| Self-hosted runners | Already configured (per spec.md). No change needed. |
| Test timeout | All CI jobs have `timeout-minutes` set. No change needed. |
| Mock regeneration | After migration, all `mock_*_test.go` files need `go generate ./...` to regenerate against V5 interfaces. |
| Build verification | CI must verify `go build ./...` passes at each phase gate. |
| Test matrix | Consider running both V4 (main branch) and V5 (migration branch) tests in parallel during migration. |

### 5.5 Branch Strategy

**Recommended approach:**
1. Create long-lived `akita-v5-migration` branch from `main`
2. Each phase is a series of PRs into the migration branch
3. Phase gates: CI must be green before advancing to next phase
4. Final merge: migration branch → `main` after Phase 5 validation
5. If a local `replace` directive is needed during development, keep it local-only and remove it before final merge

### 5.6 Documentation and Knowledge Prerequisites

- All migration engineers should read the upstream Akita repository's `migration.md` (876 lines, 11 sections)
- The V5 component model (Spec+State+Ports+Middleware+Hooks) is a significant paradigm shift — training/knowledge transfer recommended before Phase 4
- The in-place state update model (§5 of migration.md) affects how components manage mutable data

### 5.7 Blocking Issues Summary

| Issue | Status | Blocks Phase |
|---|---|---|
| #449 — gfx803/R9Nano removal | Should complete | P0 (reduces surface) |
| Akita V5 beta release | Required | P1 (module dependency) |
| V5 tracing/daisen compatibility | Required | P3 (monitoring replacement) |
| V5 cache builder API stability | Required | P4 (system wiring) |
| V5 control protocol API stability | Required | P3 (protocol rewrite) |

---

## Appendix A: File Count by Subsystem

| Subsystem | Go Files (total) | Files with akita/v4 | % Affected |
|---|---:|---:|---:|
| `amd/timing/cu/` | 48 | 23 | 48% |
| `amd/timing/cp/` | 35 | 22 | 63% |
| `amd/driver/` | 31 | 17 | 55% |
| `amd/samples/runner/` | 12 | 11 | 92% |
| `amd/emu/` | 51 | 11 | 22% |
| `amd/timing/rob/` | 5 | 5 | 100% |
| `amd/timing/rdma/` | 4 | 4 | 100% |
| `amd/timing/mem/` | 7 | 4 | 57% |
| `amd/timing/pagemigrationcontroller/` | 4 | 3 | 75% |
| `amd/timing/wavefront/` | 4 | 3 | 75% |
| `amd/protocol/` | 3 | 2 | 67% |
| `amd/sampling/` | 2 | 2 | 100% |
| `amd/benchmarks/` | 140 | 4 | 3% |
| `amd/tests/` | 4 | 2 | 50% |
| `amd/kernels/` | 6 | 1 | 17% |
| `nvidia/` | 38 | 14 | 37% |
| Root (`go.mod`) | 1 | 1 | 100% |
| **Total** | **498** | **128** | **25.7%** |

## Appendix B: Control Protocol Message Flow

```
Driver
  ├── FlushReq → GPU (CP)
  │     ├── cache.FlushReq → L1V, L1S, L2 caches
  │     ├── cache.FlushReq → TLBs
  │     ├── FlushReq → idealmemcontroller
  │     ├── CUPipelineFlushReq → CUs
  │     └── FlushRsp ← (all above)
  ├── GPURestartReq → GPU (CP)
  │     ├── RestartReq → caches, TLBs, mem controllers
  │     ├── CUPipelineRestartReq → CUs
  │     └── GPURestartRsp ← (all above)
  └── RDMARestartReq → RDMA engines
        └── RDMARestartRsp ← RDMA engines
```

All of the above separate message types collapse into `ControlReq{Command: ...}` / `ControlRsp{Command: ...}` in V5.
