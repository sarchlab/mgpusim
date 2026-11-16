# Runtime and benchmark steps

The AMD host API belongs to `amd/runtime.Runtime`, a named simulation resource. A runtime
owns contexts, stream queues, allocations, submitted commands, CPU buffer addresses,
and timestamp events. `driver.Comp`, always named `Driver`, owns only simulated
execution and protocol state. Its immutable `Spec` describes GPU destinations,
frequency, and copy timing. `RuntimeSpec` independently owns page size, allocator
selection, and device memory/CU properties. Driver reads those shared settings
through its Runtime resource. Both objects use the same page table and GlobalStorage; all live
resources are supplied before building Driver. Checkpoints restore mutable state
without comparing the rebuilt configuration.

A platform builder registers hardware and resources with `Simulation`. It
returns the Runtime. The command-line `sample.Config` composes a platform and
reporting; `runner.Runner` manages only the collection of benchmark resources:

```go
config := sample.ParseConfig()
simulation, runtime, finish := config.Build()
r := runner.New(simulation, runtime)
b := fir.NewBenchmark(runtime)
b.Length = 256
config.Configure(b)
r.AddBenchmark(b) // registers Benchmark[0]
if err := simulation.Initialize(); err != nil { panic(err) }
if err := r.Run(); err != nil { panic(err) }
if err := finish(); err != nil { panic(err) }
```

Each `Step() error` performs a finite host phase and returns. `IsEnded()` reports
whether its last host phase has completed. Runner first drains the simulation,
checks that every queue's unfinished count is zero, calls each active benchmark
in registration order, and schedules Driver once after the whole round. It
repeats until every benchmark has ended and submitted work has drained. There
is no driver goroutine or benchmark goroutine in this path. A serial engine
therefore executes the benchmark/driver/simulation loop on one goroutine.

## Queues, CPU storage, and events

Create a queue after selecting its GPU. Selection is captured by that queue.
Independent benchmarks use distinct queues; dependent DNN layers share their
operator's queue. Handles and buffer addresses survive restore.

Both H2D and D2H copy between addresses. CPU buffers occupy CPU pages in the
same checkpointed GlobalStorage resource as GPU memory. Host reads/writes
encode/decode Go values without advancing simulated time:

```go
ctx := runtime.Init()
runtime.SelectGPU(ctx, 1)
queue := runtime.CreateCommandQueue(ctx)
ptr := runtime.AllocateMemory(ctx, uint64(len(input)*4))
host := runtime.AllocateHostBuffer(queue, len(input)*4)
if err := runtime.WriteHost(host, input); err != nil { return err }
runtime.EnqueueMemCopyH2D(queue, ptr, host.Address, host.ByteSize)
runtime.EnqueueLaunchKernel(queue, code, grid, group, &args)
```

H2D reads CPU storage when it executes. **Keep its source allocated and unchanged
until completion.** Keep a D2H destination allocated, and do not read, overwrite,
reuse, or free it until completion. Runtime does not enforce occupancy or detect
conflicting host accesses; callers own lifetime and ordering. A premature serial
host write can deterministically change what an already-enqueued H2D reads.
The current synchronization point is the global simulation drain between Steps.

After a drain, the buffer can be reused for output:

```go
runtime.EnqueueMemCopyD2H(queue, host.Address, ptr, host.ByteSize)
// Return from Step; a later Step follows the global drain.
if err := runtime.ReadHost(host, output); err != nil { return err }
if err := runtime.FreeMemory(host.Context, host.Address); err != nil { return err }
```

ReadHost is repeatable and does not consume memory. There is no Runtime result
mailbox. Partial D2H responses write their destination pages directly, so a
mid-copy checkpoint saves those bytes along with the remaining protocol work.
Timing platforms retain their DMA/cache/request path; CPU storage access does
not turn timing-mode copies into magic copies.

`benchmarks.Transfers` saves upload staging descriptors and releases them in the
next drained Step. Its Upload helper explicitly copies Go data into owned CPU
storage before enqueuing. Download returns a saved CPU-buffer descriptor; Read
decodes and frees it after completion. Raw Runtime copy APIs always take
addresses. Kernel code/arguments retain enqueue-time snapshot semantics through
Runtime-owned CPU staging, released after its upload completes. Per-launch GPU
argument/packet lifetime remains unchanged. Kernel arguments are passed by
pointer, including a pointer to raw argument bytes.

`EnqueueRecordEvent(queue)` returns a zero-duration marker handle. EventTime is
repeatable after triggering. Call ReleaseEvent after consuming statistics.
Releasing a pending event invalidates its handle but does not cancel queued
work; completion cannot recreate its timestamp. Released/unknown handles return
errors and do not require persistent tombstones. Markers have no special
start/end meaning; benchmarks compute intervals from their timestamps.

## Verification and memory lifetimes

`SetVerification(false)` disables CPU comparisons and verification-only copies.
CLI `-verify` controls it, and DNN's existing `-enable-verification` also enables
operator checks. Normal outputs, training loss, and control-flow results remain
part of the workload. DNN checks are saved input/output readback descriptors,
processed in the next host Step; they do not launch a private engine loop.

DNN tensor `Free` invalidates the host tensor immediately and records the
allocation address. Physical release occurs in `GPUOperator.CompleteStep`,
after that operator's submitted batch has drained. Pending addresses are part
of the benchmark checkpoint. Tensor `Vector` requires a completed explicit
readback; mutations invalidate cached host values, including aliased views.

Cross-GPU MCCL collectives are explicit saved phase objects. Each `Step(runtime)`
enqueues one communication stage; the owning benchmark waits for the global
simulation drain before advancing the next stage.

## Direct saved state

A benchmark uses one serializable `State` for execution. The common
`benchmarks.Checkpoint[State]` supplies SaveCheckpoint/LoadCheckpoint;
there is no second snapshot struct or field-copy synchronization pass.
The embedded state pointer aliases the exact state owned by Checkpoint:

```go
type State struct {
    benchmarks.Progress
    Length int
    Queue runtime.CommandQueue
    Output []float32
}
type Benchmark struct {
    *State
    benchmarks.Checkpoint[State]
    runtime *runtime.Runtime // transient binding, excluded from State
}
func NewBenchmark(rt *runtime.Runtime) *Benchmark {
    b := &Benchmark{State: &State{}, runtime: rt}
    b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
    return b
}
func (b *Benchmark) Step() error {
    if err := b.Prepare(); err != nil { return err }
    // Normalize defaults and advance b.State directly here.
    return nil
}
```

Prepare reports a prior binding failure before the next Step. Loading decodes
before replacing state; it does not compare configuration. The state address remains stable, and the typed `bind`
hook reconnects kernel/runtime/interface references without allocation or command
submission. Nested state can be replaced, so these bindings are rebuilt even
when loading into an already-used benchmark. A failed binding prevents execution
until a successful load, rather than leaving a partially bound runnable object.

DNN operator state contains the canonical live tensor registry. Layers,
optimizers and trainer continuations store tensor IDs, so two references to the
same tensor resolve to the same object after binding. Device views have distinct
IDs and share the operator mutation epoch for host-cache invalidation. Free
retires a tensor ID immediately while pending physical frees retain its device
address until the batch drains. Replaced CPU-only layer reference slots are
retired too. Layer/optimizer states and dataset cursors are the actual objects
used by execution, linked into the saved trainer graph once at construction;
checkpointing does not assemble a parallel graph.

## Checkpoint boundaries and migration

A checkpoint is safe after **every benchmark has returned from the current
round and Driver's tick has been scheduled**. `Runner.SaveCheckpoint` enforces
this boundary. Direct callers of `Simulation.SaveCheckpoint` must honor the
same host-phase precondition. Serial `RunUntil` may advance from that boundary to an event
boundary inside simulation before saving. Never save in the middle of a host
Step or between two benchmarks' Steps. Both save and load require an initialized,
idle serial simulation; Akita enforces this at the Simulation boundary. Runtime
resource serializers do not own an engine and are not independent concurrent APIs.
Loading into an already-used simulation intentionally replaces its saved State,
allowing same-instance rewind; it must not reject existing queues or allocations.
The same host-phase boundary applies before loading. Across executables or changed
configuration, loading is best effort, with no version or configuration-hash gate.

A fresh process builds the same platform and named benchmark resources with the
same requested configuration, initializes Simulation, and loads the checkpoint.
Do not run benchmark setup or replay submission before loading. `Runner.Run`
first drains saved simulation work before calling the next benchmark Step.
Each benchmark saves its phase, host data, handles, and deterministic RNG state.
DNN saves layer activations/views, optimizer histories, data cursors, deferred
frees, and pending checks. Small synthetic datasets stay embedded. MNIST/CIFAR
save file identity, SHA-256 integrity metadata, shape, descriptor and cursor;
bind validates the actual files and reloads normalized host data, sharing an
unchanged dataset across GPU sources. Missing or changed files fail restoration,
even if this process previously cached them. Lazy ImageNet input files must
remain unchanged and available at the same configured root. No active file
handles are serialized or advanced, and binding does not enqueue GPU work.

Generic JSON checkpoint fields still cannot represent NaN or infinity. Support
is deferred to [Akita #573](https://github.com/sarchlab/akita/issues/573); no
MGPUSim-specific float encoding is introduced. Raw storage bytes preserve their
bit patterns, but remaining JSON float fields retain this limitation.

This is an API break: replace the former blocking Driver `Run`, `Terminate`,
`LaunchKernel`, `MemCopy*`, and `DrainCommandQueue` calls with Runtime enqueue
operations and explicit host phases. Replace result-buffer binding with saved
CPU-buffer addresses and explicit ReadHost/FreeMemory. Move platform construction/reporting out of
Runner. Runtime never owns an engine or a reference to Driver.

The global batch scheduling boundary can remove host scheduling ticks that the
old blocking/goroutine path inserted. Simulated task durations may therefore
shift slightly even though device latency settings are unchanged; deterministic
repeated runs are required, rather than bitwise timing parity with the old loop.
KMeans requires a positive `MaxIter`; nonpositive limits return an error before
allocation or command submission.

The VGG16 checkpoint regression constructs the real full layer/parameter/view
graph and uses a synthetic ImageNet image plus bounded first-layer transfers.
It seeds the trainer at epoch completion to test wrapper serialization and
fresh-process restore without allocating giant randomization buffers or running
full 224×224 training. Separate small-network tests exercise convolution,
max-pooling, fully connected layers, Adam and multi-GPU collective recovery.
This bounded graph test does not establish full VGG16 training accuracy.

The former `amd/server` HTTP API and `amd/samples/server` executable have been
removed. Compose a simulation and Runtime directly as shown above. Akita
monitoring and the instruction debugger are unaffected. Runtime handles and
command types now live in `amd/runtime`; there are no `amd/driver` aliases.
