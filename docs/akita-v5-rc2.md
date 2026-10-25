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
memory, page tables, dispatchers, compute-unit runtime graphs and sampling state
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

D2H submissions return command IDs. Benchmarks save those IDs in their own
continuation and retrieve retained bytes with `Runtime.ReadResult` in a later
Step after the global simulation drain. No output pointer binding is needed.
A failed read leaves the result available; a successful read consumes it.
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
