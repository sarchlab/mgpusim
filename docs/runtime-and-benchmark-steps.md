# Runtime and benchmark steps

The host API belongs to `driver.Runtime`, a named simulation resource. A runtime
owns contexts, stream queues, allocations, submitted commands, readback bytes,
and timestamp events. `driver.Comp`, always named `Driver`, owns only simulated
execution and protocol state. Its immutable `Spec` describes GPU destinations,
frequency, and copy timing. `RuntimeSpec` independently owns page size, allocator
selection, and device memory/CU properties. Driver reads those shared settings
through its Runtime resource. Both objects use the same page table; all live
resources are supplied before building Driver. Their checkpoint configuration
checks follow these separate ownership boundaries.

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
External HTTP requests are serialized by the server, which drives the same
queue and simulation operations at the request boundary.

## Queue and result handles

Create a queue after selecting its GPU. Selection is captured by that queue.
Independent benchmarks use distinct queues; dependent DNN layers share their
operator's queue. Handles identify Runtime-owned objects and survive restore.

```go
ctx := runtime.Init()
runtime.SelectGPU(ctx, 1)
queue := runtime.CreateCommandQueue(ctx)
ptr := runtime.AllocateMemory(ctx, uint64(len(input)*4))
runtime.EnqueueMemCopyH2D(queue, ptr, input)
runtime.EnqueueLaunchKernel(queue, code, grid, group, &args)
```

H2D bytes, kernel arguments, code, and dispatch dimensions are captured when
enqueued. A later host mutation cannot change submitted work. Kernel arguments
are passed by pointer (including a pointer to raw argument bytes).

After the execution phase drains, another Step can request D2H:

```go
result := runtime.EnqueueMemCopyD2H(queue, ptr, len(output)*4)
```

Save `result` in the benchmark's continuation. A later Step calls
`runtime.ReadResult(result, output)` to decode and consume the retained bytes.
It never waits or runs the engine. An unavailable result or incorrect destination
returns an error; failed reads do not consume the result. Host pointers and
callbacks are not retained by Runtime. Data-dependent algorithms such as BFS
and KMeans perform mandatory control-flow readbacks even with verification off.

`EnqueueRecordEvent(queue)` returns an event handle. `EventTime(handle)` returns
its trigger time after completion. A marker adds no simulated duration and has
no special start/end meaning; benchmarks compute intervals from timestamps.

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
`benchmarks.Saved[Config, State]` supplies SaveCheckpoint/LoadCheckpoint;
there is no second snapshot struct or field-copy synchronization pass.
The embedded state pointer aliases the exact state owned by Saved:

```go
type State struct {
    benchmarks.Progress
    Length int
    Queue driver.CommandQueue
    Output []float32
}
type Config struct { Length int; Verify bool }
type Benchmark struct {
    *State
    benchmarks.Saved[Config, State]
    runtime *driver.Runtime // transient binding, excluded from State
}
func NewBenchmark(rt *driver.Runtime) *Benchmark {
    b := &Benchmark{State: &State{}, runtime: rt}
    b.Saved = benchmarks.NewSaved(b.State,
        func() Config { return Config{b.Length, b.Verification} },
        b.bind)
    return b
}
func (b *Benchmark) Step() error {
    if err := b.Prepare(); err != nil { return err }
    // Normalize defaults and advance b.State directly here.
    return nil
}
```

Prepare captures requested configuration before a first Step normalizes defaults;
a pre-Step Save or Load also captures it. Loading checks configuration and decodes
before replacing state. The state address remains stable, and the typed `bind`
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
Step or between two benchmarks' Steps.

A fresh process builds the same platform and named benchmark resources with the
same requested configuration, initializes Simulation, and loads the checkpoint.
Do not run benchmark setup or replay submission before loading. `Runner.Run`
first drains saved simulation work before calling the next benchmark Step.
Each benchmark saves its phase, host data, handles, and deterministic RNG state.
DNN saves layer activations/views, optimizer histories, data cursors, deferred
frees, and pending checks. Resident datasets are saved; lazy ImageNet input files
must remain unchanged and available at the same configured root. No active file
handles are serialized or advanced during restore.

This is an API break: replace the former blocking Driver `Run`, `Terminate`,
`LaunchKernel`, `MemCopy*`, and `DrainCommandQueue` calls with Runtime enqueue
operations and explicit host phases. Replace result-buffer binding with saved
command handles and `ReadResult`. Move platform construction/reporting out of
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
