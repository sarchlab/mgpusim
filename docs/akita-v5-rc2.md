# Akita v5 RC4 migration and checkpoints

MGPUSim uses the published `github.com/sarchlab/akita/v5 v5.0.0-rc.4`
release, including checkpoint restoration without configuration hashes. Akita
guarantees each requested `TickAt` clock edge and stores the pending tick guard as a list. Recording tables
now have the `akita$` prefix; MGPUSim's
own metric tables are unchanged. No local module replacement is required. AMD GCN3,
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
memory, page tables and sampling state
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
vector-memory pipelines, and the time-ordered `SampledCompletions` list.
Queue, pipeline, and sampled-completion references use
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

Sampled waves append `{At, Wave}` to State and call `TickAt(At)` once. Each tick
completes due waves before pipeline work, ends their tasks, and retires drained
groups. It also drains newly due work after input, so a zero-duration prediction
finishes on the current edge. There is no custom completion/dispatch event,
additional engine handler, or engine resource on the CU. Future entries keep
their original wakeups without re-requesting them. Completion-port backpressure
retains the completed group and retries on a normal tick.

`TickAt` rounds up to the first CU clock edge at or after the predicted time.
This can move sampled completions by less than one period compared with RC2's
unaligned engine events. `TestSampledFIRCheckpointFreshProcess` trains the sampler
with a controlled history (FIR256 alone is below its warmup threshold), saves
with sampled waves pending, and checks exact aligned completion times and an
identical final archive after a fresh-process restore. The first kernel uses
sampling, so this test does not assert its numerical output; the existing
unsampled GCN3/CDNA3 tests verify FIR output independently.

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
`wavefront.Handle` is a distinct local-slot type used by saved wave references
and pending sampled-completion records. Global kernel wave UIDs, instruction IDs
and task IDs remain separate `uint64` values.
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
encodings and dynamic IDs remain in State. `wavefront.Inst` is removed: `Inst()`
returns a shared read-only `*insts.Inst` description and `InstID()` returns the
separate dynamic execution identity. Async records and tracing carry the ID.
The pure built-in decode cache is keyed by full encoding, decoder instance
(which owns its fixed ISA tables), CDNA3 mode, and the canonical stored PC.
Hits reuse the description without copying or allocating. The stored PC semantics
are unchanged; execution uses the wave's PC. Custom decoders still bypass the
cache because their interface does not declare purity or configuration identity.
Callers must not modify descriptions or their operands/formats.

Code-object metadata is borrowed directly from the current group's canonical
`CodeObjectMetadata` for each dispatch, register reset, or debugger operation.
Executable bytes live at `Packet.KernelObject` in GPU storage within the request
process address space; launch messages also carry only metadata. The
borrow includes version and metadata; required missing metadata fails fast.
No code object is reconstructed or cached. Callers must not mutate the value or
retain its pointer beyond the immediate operation. State replacement therefore
changes subsequent reads immediately without invalidation or rebinding.
Tracing detail retains wave/instruction IDs and format metadata, so debugger
observers do not keep a second execution graph alive across ticks.

The instruction and scalar memory destinations are immutable `Spec.InstMem` and
`Spec.ScalarMem`. A platform builder first builds the instruction/scalar ROBs and
binds their ports, then obtains their actual remote addresses:

```go
spec := cu.DefaultSpec()
spec.InstMem = instructionROB.Ports.Top.AsRemote()
spec.ScalarMem = scalarROB.Ports.Top.AsRemote()
spec.VectorMemModules = mem.AddressMapper{
    Destinations: []messaging.RemotePort{vectorROB.Ports.Top.AsRemote()},
}
computeUnit := cu.MakeBuilder().WithSimulation(simulation).
    WithSpec(spec).WithResources(resources).Build(name)
// Bind the CU's local ports, then connect those ports to the ROBs.
// Initialize the simulation after the full topology has been assembled.
```

Vector routing uses
concrete `Spec.VectorMemModules` (MGPUSim #321), populated before Build. Downstream-first
construction is temporary: Akita #576 tracks a Blueprint that declares the whole
topology, instantiates it, and then connects it, including cyclic topologies.
CU-to-ROB wiring uses actual bound addresses and adds no lifecycle phase.
Cross-GPU RDMA assembly separately precomputes the declared GPU/RDMA port names
before building any GPU, so each component receives complete routing.

A checkpoint must restore exactly with the same executable on the same platform.
Across executables or configuration changes, restoration is best effort: a
loadable checkpoint is used; otherwise loading reports an error. Checkpoints
have no version or configuration-hash gates.
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
`WfSampledWaiting` until Tick processes their due sampled-completion records.
They never clear detailed wave register allocations.

These state transitions replace CU-wide searches for saved wave handles. Tests
independently inspect decoder, pipeline, SIMD, scheduler, memory and
sampled-completion records at operation boundaries. Instructions record their
owning local slot and whether execution is pending. An asynchronous instruction retains its encoding
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


## DMA package and command processor ownership

The DMA component now lives in `amd/timing/dma`. Replace `cp.DMAComp`,
`cp.DefaultDMASpec`, and `cp.MakeDMAEngineBuilder` with `dma.Comp`,
`dma.DefaultSpec`, and `dma.MakeBuilder`. Its processing behavior and port names
are unchanged. DMA routing now uses concrete `Spec.LocalDataSource` (#321);
populate its destinations before Build instead of injecting a Resources mapper.

CP peer addresses and CU capacities belong to `cp.Spec`. Build DMA, RDMA,
compute units and caches first, then collect their actual bound addresses.
`cp.DescribeCU(cu.DispatcherView{CU: component})` produces one immutable CU record;
its position in `Spec.CUs` defines the dispatcher ordering. Supply Driver,
DMAEngine, RDMA and cache control destinations before CP Build. The old
`WithDriver`, `WithCU`, and `RegisterCU` interfaces are removed. Unused DRAM
control addresses are no longer stored on the CP; assembly still binds their
ports for lifecycle validation.

Each CP owns its dispatcher cursors, pending dispatch locations, task handles,
per-die rate gates, and shared CU reservations in `State`. Middleware Initialize
constructs behavior views that resolve the current State on every operation,
including after LoadCheckpoint replaces it. Grid and work-group values persist
without cyclic pointer graphs. Allocation masks include unlimited allocation
cursors. Dispatchers and CU pools are no longer separately registered resources.
Sampler and monitor references are supplied through Resources before Build.

State schemas and entity inventories can change across builds; restoration across
those builds is best effort. Tests compare fresh-process continuation
for all four dispatch algorithms with two active kernels, occupied shared CU
slots and a blocked output port, including complete final archives, request
IDs and simulated times. Loading does not compare the rebuilt Spec with the
saved configuration.
