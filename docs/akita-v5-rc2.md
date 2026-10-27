# Akita v5 RC2 migration and checkpoints

MGPUSim uses the published `github.com/sarchlab/akita/v5 v5.0.0-rc.2`
release. RC2 includes the scoped task-notification index and redundant-map
cleanup from [Akita PR #567](https://github.com/sarchlab/akita/pull/567), fixing
rc1's reporting performance regression without changing its APIs or checkpoint
format. No local module replacement is required. AMD GCN3,
AMD CDNA3 and the NVIDIA trace simulator use native Akita components, ports,
message envelopes, task tracking and checkpoint resources. Platform frequencies,
cache geometry, dispatch policies, instruction semantics and sample CLI flags
are preserved. See the validation notes below for dependency timing differences.

Explicit AMD `-metric-file-name` and NVIDIA `-output` names keep beta.10's
base-name convention: `.sqlite3` is always appended, and a rerun replaces that
recording. NVIDIA uses `-output` only with visualization tracing enabled.

## Message and construction APIs

Messages are `messaging.Msg` envelopes with a pure value `Payload`. Routing and
request identity belong to the envelope. Code objects, dispatch packets,
work-groups and NVIDIA traces cross component boundaries as values. Work-group,
wavefront and instruction IDs come from the owning simulation; a receiver
reconstructs its own runtime graph. No message contains a Go pointer or callback.

`kernels.WGSelection` replaces `WGFilterFunc`. Its zero value selects the entire
grid. A partition is `{Partitioned: true, First: first, End: end}`, selecting the
half-open interval `[first, end)` of flattened work-group indices, with X varying
fastest, then Y and Z. An empty partition remains empty. The existing unified-GPU,
partition and per-die dispatch policies use this representation. Arbitrary Go
closures are no longer accepted by the launch/grid APIs.

Custom platforms must build with a simulation, bind each port to its component
before connecting it, and call `Simulation.Initialize()` after wiring. Shared
memory, page tables, dispatchers and sampling state
are registered resources. Sampling history belongs to one simulation. Existing
sample main functions explicitly initialize the composed simulation.

## Save and resume submitted work

Build the same platform configuration in the restoring process, including GPU
count/model, architecture, component/resource names, memory layout and tracking
options. Initialize it, then call `Simulation.LoadCheckpoint(path)`. Do not reload
traces or resubmit commands: submitted kernels, queued commands, host-to-device
source bytes and in-flight memory operations are already in the checkpoint.

AMD host work uses the checkpointed Runtime and benchmark Step loop described in
[Runtime and benchmark steps](runtime-and-benchmark-steps.md). Save only after
an entire host round has returned and the Driver tick has been scheduled.
`Runner.SaveCheckpoint` enforces that boundary. Lower-level direct callers must
honor the same precondition. A serial engine may run to an intermediate event
boundary with `RunUntil` before saving.

D2H submissions write to caller-owned CPU storage described by a `HostBuffer`.
Benchmarks save that descriptor in their continuation and decode the buffer with
`Runtime.ReadHost` in a later Step after the global simulation drain. Reads are
repeatable; callers explicitly release the allocation with `Runtime.FreeMemory`.
Alternatively, `benchmarks.Read` decodes the buffer and releases it after a
successful read. No output pointer binding is needed.
Fresh-process reconstruction registers the same named workload resources before
loading; `Runner.Run` first drains the saved simulation before the next Step.
There is no asynchronous Driver host loop.

The NVIDIA trace runner has no AMD host result buffers; its trace progress,
resident warps, scoreboards and memory pipeline state resume from the archive.

The archive restores simulation state, not Go call stacks, host queue listeners,
monitor connections or application goroutines. External task observers and output
databases are not part of the MGPUSim checkpoint: an observer attached in a fresh
process sees subsequent events, not a replay of its earlier history. Sampling's
host wall-clock measurement restarts. Checkpoints are for the same program and
platform configuration; beta.10 checkpoint compatibility is not provided.

## Validation

Fresh-process tests rebuild only the topology and load a mid-kernel archive:

- AMD emulation and timing both cover GCN3 and CDNA3 with two submitted FIR
  kernels. Timing checkpoints contain vector memory requests in flight. One
  output is read after continuation; another remains retained until requested.
  All 512 output values and the complete final archive match uninterrupted runs.
- NVIDIA tests stop at queued, resident-warp and memory-in-flight stages, then
  finish two submitted kernels. They verify 544 instructions, 32 warps and an
  identical final archive, including event time and component state.
- Additional tests cover shared LDS/barrier/pool identities, pipeline progress,
  allocator free-list and unified-GPU continuation, sampling prediction windows,
  serializable partition cursors and invalid result destinations.

Fresh-process checkpoint tests build full GPU simulations, can take several
minutes, and require several GiB of memory (especially CDNA3/MI300X). They skip
under `go test -short`; normal CI continues to run them without `-short`. The
uninterrupted simulation is terminated and its heap released before starting
the restore subprocess, so the two simulation heaps do not overlap. On
memory-constrained machines, use `-p 1` to avoid running multiple checkpoint
packages concurrently.

Run these tests with:

```sh
go test -p 1 ./amd/samples/runner/emusystem ./amd/samples/runner/timingconfig ./nvidia/runner -run Checkpoint -v
go test ./amd/driver/... ./amd/kernels ./amd/sampling ./amd/timing/cu
```

### Timing comparison with beta.10

The dependency upgrade changes TLB timing: beta.10 used
`Pipeline.AcceptWithDelay(..., 1)`, while rc1 uses `Pipeline.Accept(...)` and
removes that extra dwell cycle. MGPUSim retains its configured TLB latency and
width; this migration does not add compensating latency or retune the GPU.

rc1 also removes the `CanWaitForCoalesce` request field. In beta.10 the address
translator only copied this field; no memory component used it to schedule
requests. Completion still waits for every transaction of an instruction, so
out-of-order responses cannot release its registers early.

An isolated comparison against main `62a691424fe3bcde9c3815bc6dd65ed5b95f16d2`
queued two identical 256-element FIR kernels before starting the engine. All
outputs matched. Final simulation times were:

| Architecture | beta.10 | rc1 | rc1 with old TLB dwell, diagnostic only |
| --- | ---: | ---: | ---: |
| GCN3 | 17,507,000 ps | 17,480,000 ps | 17,507,000 ps |
| CDNA3 | 13,442,000 ps | 13,432,000 ps | 13,442,000 ps |

The diagnostic used a separate temporary Akita copy and module file; neither is
part of this migration. This isolates the dependency's timing change from
MGPUSim execution. A separate GCN3 sample comparison also matched all 857 metric
rows after restoring the old TLB dwell in that diagnostic copy. Host-driven
samples can additionally start commands at different engine boundaries, so the
prequeued comparison is the controlled timing check.

CPI stack durations are accumulated in integer picoseconds and converted to
floating-point cycles only when reporting. This removes event-segmentation
rounding noise observed by the exact-equality CI check; it does not change
instruction scheduling or attributed intervals. The earlier 857-row diagnostic
above used the previous floating-point accumulator. Its last few CPI digits may
differ from the final reporter, while counts and simulation times are unchanged.

## Compute-unit State and construction

A timing CU is one ticking component. Its `State` owns work-group and wavefront
execution values, shared LDS, scoreboards, register bytes, instruction encodings,
in-flight memory records, scheduler queues, decoder progress, SIMD slots, and
vector-memory pipelines. Queue, pipeline, and sampled-event references use
CU-local slot handles containing a slot index and generation. Global wave UIDs
remain the tracing and protocol identity.
There is no CU Runtime resource or per-SIMD checkpoint resource, and saving does
not collect a second execution graph. Finished instructions are retired when
neither wavefronts nor in-flight requests reference them. Execution completion
stops further instruction fetch. A terminal wave keeps
its original hardware slot until dependent requests and pipeline work drain.
The work-group completion message releases CP resources only after this drain;
a full completion port retains the slots and schedules a retry. This is a
conservative modeling choice, so workloads with late fetch replies may finish
later than with early CP release. There is no retired-wave spill storage.

The work-group's final S_ENDPGM task closes before its wavefront task; completion
port retries do not replay either completion. The previous final-wave completion
branch closed the wavefront first, omitting the last S_ENDPGM interval from some
CPI Special buckets. Including this interval
can change those buckets without changing instruction counts or simulation time.

`Middlewares.Initialize` creates the scheduler and execution-unit behavior.
Wavefront and work-group views live in the `cu` package and carry only their CU
owner, slot handle, and global UID. `State.Initialize` allocates fixed-length
`State.Waves` and `State.Groups` arrays, each bounded by `numWfPools * WfPoolSize`.
Retirement clears occupancy without resizing or moving slots. Wave state links
directly to its group slot; generation checks reject stale recycled handles.
Scheduler ordering is still defined by its membership queues.
Replacing State therefore requires no invalidation sentinel, rebinding callback,
load hook, or subsequent tick. Previously acquired ID views and register-file
accessors see the replacement immediately, as long as the referenced ID remains
live. Temporary pointers and slices returned internally are used only during the
operation that obtained them.

Pools, barriers, pipelines, and memory requests resolve the same canonical wave
values; all waves of a group share its canonical LDS. The ALU borrows LDS only
for an execution call and releases that reference before returning. ALU and
coalescing operations resolve a call-local state/register adapter once for all
lanes and retain no state references after the call. Instruction
encodings remain in State; decoded instructions are temporary operation values.
A pure decode cache for the built-in disassembler is keyed by the full instruction
encoding, decoder instance (which owns its fixed ISA tables), and CDNA3 mode. Custom
decoders bypass it. Cached descriptions are immutable; PC and dynamic IDs are
attached to local copies, and no State-dependent identity is cached.
Tracing detail retains wave/instruction IDs and format metadata, so debugger
observers do not keep a second execution graph alive across ticks.

The instruction and scalar memory destinations are immutable `Spec.InstMem` and
`Spec.ScalarMem`. A platform builder first builds the instruction/scalar ROBs and
binds their ports, then obtains their actual remote addresses:

```go
spec := cu.DefaultSpec()
spec.InstMem = instructionROB.Ports.Top.AsRemote()
spec.ScalarMem = scalarROB.Ports.Top.AsRemote()
computeUnit := cu.MakeBuilder().WithSimulation(simulation).
    WithSpec(spec).WithResources(resources).Build(name)
// Bind the CU's local ports, then connect those ports to the ROBs.
// Initialize the simulation after the full topology has been assembled.
```

Changing either destination changes the checkpoint Spec hash. Routing through
`Resources.VectorMemModules` is deferred to MGPUSim #321. Downstream-first
construction is temporary: Akita #576 tracks a Blueprint that declares the whole
topology, instantiates it, and then connects it, including cyclic topologies.
This CU change does not add a lifecycle phase or predict port names.

Checkpoint compatibility with previous implementations is not supported. Rebuild
and resume with the same program and configuration; this also applies after v5.
SIMD task names and timing remain unchanged, while reporting routes their task
notifications through one simulation observer rather than registering the SIMD
behavior views as simulation entities.

### CU execution and retirement

A detailed wave stops fetching when it reaches `S_ENDPGM`, including while it
waits for outstanding scalar/vector memory instructions. After execution ends,
`WfDraining` retains the original hardware slot while an already-issued prefetch
returns. `WfCompleted` means fully drained and waiting for the group's completion
message. The CU sends that message and releases slots only when the dispatch
port accepts it; backpressure schedules a retry. Barrier peer checks distinguish
execution-ended waves from waves still executing. Sampled waves instead remain
`WfSampledWaiting` until their scheduled completion event and never clear detailed
wave register allocations.

These state transitions replace CU-wide searches for saved wave handles. Tests
independently inspect decoder, pipeline, SIMD, scheduler, memory and custom-event
holders at operation boundaries. Instructions record their owning local slot and
whether execution is pending. An asynchronous instruction retains its encoding
through its last memory response even after its wave advances; completion
observers finish before the encoding can be retired. Tracing does not control
this lifetime. No per-holder reference counts or retired-wave spill storage are
used.

The old CU shadow buffers, flush/restart/replay API, and CP GPU shootdown/restart
sequence are removed. Ordinary driver cache flush, kernel-start L1 invalidation,
and independent RDMA control remain. Future page migration must drain already
issued accesses without replaying CU execution. This change does not implement
that migration protocol. There is no backward checkpoint compatibility contract,
including after the v5 release.
