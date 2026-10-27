# Timing-model tracing / instrumentation

MGPUSim builds its simulation through Akita's `simulation.Simulation`, which is
the registrar for every component and port. `RegisterComponent` auto-attaches
the vis `DBTracer` (`CollectTrace`) and `RegisterPort` auto-attaches the
incoming- and outgoing-buffer tracers. So every timing component — the CU, the
Command Processor, the DMA engine, the dispatchers, the RDMA engine — and the
reused Akita `mem/` hierarchy (caches, TLBs, address translators, ROBs, DRAM)
are traced when `-trace-vis` is on, and every port gets free buffer-queueing
tasks. This document covers the **MGPUSim-authored** instrumentation; the
`mem/` components are instrumented inside Akita.

Requires Akita **v5.0.0-beta.7** or later (the buffer-admission registry
`MsgIDAtIncomingBuffer`, the reset helpers `EndReqInOnReset`/`EndTaskOnReset`,
and `MilestoneKindWork`).

## Task lifecycle

| Component | req_in | req_out | internal tasks |
|---|---|---|---|
| CU | MapWGReq (per work-group) | inst fetch, scalar/vector mem | `wavefront`, `inst`, `fetch`, SIMD `pipeline` |
| Command Processor | LaunchKernelReq, MemCopy, FlushReq | cloned MemCopy → DMA | — |
| DMA | MemCopy | per-unit Read/Write → mem | — |
| Dispatcher | — | MapWGReq → CU | — |
| RDMA | forwarded request | forwarded clone | — |

### Execution completion

CU execution uses `beginInstruction` / `completeInstruction`, including final
memory responses. Completion callbacks finish before the instruction encoding
can be retired. Instruction task `Detail` contains `inst` (dynamic instruction ID),
`wf` (global kernel wave UID), and `format`; it holds no execution wrapper or
State pointer. The debugger resolves the ID while the completion callback runs.
Decoded descriptions may be shared across dynamic IDs, so pointer identity is
not execution identity. Local `wavefront.Handle` values identify CU slots and
are distinct from these tracing IDs. The final `S_ENDPGM` task closes before the wave task; completion
port retries do not repeat either task completion. Sampled waves close their
wave tasks when Tick processes their due sampled-completion records. The CU
retains hardware slots until outstanding prefetches drain and the work-group
completion message can be sent.
There is no CU flush/replay teardown. See `slots_test.go` and `lifecycle_test.go`
for completion ordering, tracking-disabled execution, and backpressure coverage.
The CP's ordinary cache `FlushReq` req_in closes when the flush-done response is sent.

## Milestones (blocked-time attribution)

Milestones mark when a blocking condition resolves; the interval from the
previous milestone (or task start) to a milestone is time spent blocked on that
reason. Admission waits hang on the port's incoming-buffer task (via
`MsgIDAtIncomingBuffer`); processing waits hang on the req_in / inst task.

| Component | Milestone | Kind | Marks |
|---|---|---|---|
| DMA | `processing slot` | hardware_resource | waited for a concurrency slot (buffer task) |
| DMA | `ToMem` | data | waited for the memory round trip (req_in) |
| CP | `dispatcher` | hardware_resource | waited for a free dispatcher (buffer task) |
| CP | `ToDMA` | network_busy | waited for the downstream DMA port (buffer task) |
| RDMA | `remote` | data | waited for the round trip to the other side (req_in) |
| CU | `vmem-inflight` | hardware_resource | waited for an in-flight vector-mem slot (inst) |
| CU | `coalesce` | work | coalescing/transaction-issue, until the first vector-mem transaction is ready to send, backed by a `pipeline` subtask (inst) |
| CU | `vmem` / `smem` | data | waited for the vector/scalar memory response (inst) |
| CU | `s_waitcnt` | data | S_WAITCNT waited for outstanding memory (inst) |
| CU | `s_endpgm` | data | S_ENDPGM drained outstanding memory (inst) |

A vector-memory `inst` task is tiled into three phases: `vmem-inflight`
(`hardware_resource`, the in-flight slot is acquired at execute) → `coalesce`
(`work`, until the first transaction is ready to send) → `vmem` (`data`, the
last response returns, at the task end). The `coalesce` interval is the unit
*doing work*
(coalescing + transaction-issue pipeline), not blocking, so it is a `work`
milestone backed by a child `pipeline`/`coalesce` subtask that spans it (per the
"work ⇒ subtask" coverage rule). The request is not yet outstanding before the
first send, so that interval must not be attributed to the data wait — the
`data` interval then lines up exactly with the child `req_out` transaction bars.

Each component package has a `milestone_tracing_test.go` (or equivalent)
asserting its milestones with a recording tracer.

## Known limitations / intentional gaps

- **CU pre-issue stalls are not instrumented.** The scheduler's structural
  hazard (`CanAcceptWave`) and the port back-pressure (`!CanSend`) stalls happen
  *before* the instruction's task exists, so there is no task to attribute the
  wait to. The same applies to the dispatcher's free-CU and ToCUs-send waits.
- **SIMD `pipeline` tasks live on the SIMD-unit domain on purpose** — the
  per-SIMD `BusyTimeTracer` (report.go) needs them there. The cross-domain
  parent ("inst" on cu.comp) resolves because the vis DBTracer is one instance
  on both domains.
- **Control plane** (ordinary cache flush, kernel-start invalidation, and RDMA
  drain/restart in `ctrlMiddleware`) is not instrumented beyond completing the
  FlushReq req_in. The legacy GPU shootdown/restart sequence is removed.
- **Tags** are not emitted; the read/write/direction distinctions live in each
  task's `What`/`Detail`.
