# simulation — Top-Level Simulation Runner

Package `simulation` provides the top-level simulation runner for the Akita
simulation framework. It wires together an engine, a data recorder, a visual
tracer, and an optional monitoring server, and acts as a global inventory that
registers every runtime object as a named entity.

## How It Works

A `Simulation` is created through its `Builder`, which constructs and connects
the supporting infrastructure. Once built, you register components, connections,
and shared resources with the simulation; registration tracks each object in a
flat inventory and wires it into tracing and monitoring. The simulation exposes
typed accessors for the engine and infrastructure, plus name-based and
inventory accessors for the registered entities.

The runner depends only on minimal local interfaces (`Component`, `Port`,
`Connection`, `Resource`). Concrete messaging types satisfy them structurally,
so this package does not import the messaging package.

## Key Concepts

- A `Simulation` holds an `Engine` (serial or parallel), a SQLite-backed
  `DataRecorder`, a `DBTracer` for visual tracing (used by the Daisen
  visualizer), and an optional `Monitor` for live web inspection.
- Every registered runtime object — component, port, connection, and
  shared-state resource — satisfies the `Entity` interface and is held in one
  flat inventory. Entity names are **globally unique across all kinds**.
- A `Resource` is non-timing program state (such as memory contents or page
  tables) that multiple components reference. The simulation owns resources;
  components hold references to them rather than embedding the payload.

## Entity Interfaces

`Entity` is the abstract base interface; each kind embeds it and adds its own
capabilities:

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `Entity` | `Name() string` | Abstract base for every registered object |
| `Component` | `Entity` | Runtime components |
| `Port` | `Entity` + `NumIncoming()` / `NumOutgoing()` | Port buffers |
| `Connection` | `Entity` | Runtime connections |
| `Resource` | `Entity` | Shared state registered by setup, reachable by name |

## Builder Pattern

```go
sim := simulation.MakeBuilder().
    WithParallelEngine().       // optional: use the parallel engine
    WithMonitorPort(8080).      // optional: monitoring server port
    WithVisTracingOnStart().    // optional: start tracing immediately
    Build()

defer sim.Terminate()
```

| Method | Description |
|--------|-------------|
| `WithParallelEngine()` | Use `ParallelEngine` instead of `SerialEngine` |
| `WithoutMonitoring()` | Disable the monitoring web server |
| `WithMonitorPort(port)` | Set the monitoring server port |
| `WithOutputFileName(name)` | Custom SQLite output file name |
| `WithVisTracingOnStart()` | Enable visual tracing from time 0 |

## Usage

### Registering Components

```go
sim.RegisterComponent(myComponent)
```

Registration automatically adds the component and its ports to the inventory,
attaches visual tracing hooks, connects the component to the monitoring system
(if enabled), and registers any shared-state resources it exposes. Use
`RegisterConnection` and `RegisterResource` to register connections and shared
resources directly.

### Accessing the Simulation

```go
engine := sim.GetEngine()
recorder := sim.GetDataRecorder()
tracer := sim.GetVisTracer()

comp := sim.GetComponentByName("myComp")    // panics if not registered
port := sim.GetPortByName("myComp.Top")     // panics if not registered

components := sim.Components()               // copy, in registration order
connections := sim.Connections()
resources := sim.Resources()
```

`GetComponentByName` and `GetPortByName` resolve a globally unique name to the
registered entity. `Components`, `Connections`, and `Resources` each return a
copy of the registered objects in registration order, which is useful for
inventory, debugging, and tooling.

## Checkpoint and Resume

A simulation can be checkpointed to a `.tar.gz` archive and resumed later — for
example, to snapshot between GPU kernels or to restart a long run. The contract
is an oracle: *running to the end* must equal *checkpoint, rebuild, restore, run
to the end*.

```go
// Save (engine stopped, outside an event handler):
err := sim.SaveCheckpoint("snap.tar.gz", "")  // "" uses the default build ID

// Resume: rebuild the *identical* simulation with the same setup code, then:
err := sim.LoadCheckpoint("snap.tar.gz", "")
engine.Run()                                   // continue to completion
```

Setup rebuilds the *shape* (components, ports, connections, resources, wiring);
the checkpoint restores only the *runtime* (each entity's `State`, port buffers,
shared resources, the event queue, the engine time, and the ID-generator
counter). The archive holds a `build_id` entry plus one payload per entity; the
payload files are the inventory (there is no manifest). Loading validates the
build ID and that the saved and rebuilt entity sets match exactly.

### The golden rule: all mutable runtime state lives in `State`

Anything that changes during simulation and is not derivable from the restored
queue **must** be a field of the component's `State` (or a registered resource).
Runtime state hidden on a middleware struct — a round-robin cursor, a counter, an
RNG — is *not* checkpointed, so a resumed run silently diverges. Keep middleware
fields to structural wiring (ports, downstream references, routing tables) that
setup rebuilds; put cursors and counters in `State`.

### Requirements and tips

- **Serial engine only.** `SaveCheckpoint`/`LoadCheckpoint` reject a
  `ParallelEngine`.
- **Run with tracing off** for a deterministic resume: the tracing task-ID side
  table consumes the global ID generator, perturbing the ID sequence.
- **`SerialEngine.RunUntil(t)`** stops the engine at a deterministic boundary
  (every event with time ≤ `t`), unlike `Run` (drains everything) or `Pause`
  (stops at a non-reproducible point) — useful for taking a mid-transaction
  checkpoint.
- **Register your message and event types.** A port can hold any `messaging.Msg`
  and the engine queue any `timing.Event`; each concrete type must be registered
  with `messaging.RegisterMsg` / `timing.RegisterEvent` in an `init()` so a
  checkpoint that captures it can be decoded. A forgotten registration fails
  loudly at load.
- An entity package becomes checkpointable by implementing the structural
  `Checkpointable` interface (`SaveCheckpoint(io.Writer)` / `LoadCheckpoint(io.Reader)`);
  it never imports `simulation`. `modeling.Component`/`EventDrivenComponent`,
  ports, `mem.Storage`, and `vm.PageTable` already do.

**Writing your own checkpointable messages, events, and components:** see
[`doc/tutorial/checkpointing.md`](../doc/tutorial/checkpointing.md) for the full checklists, a
worked example, and the gotchas.

See `examples/checkpointdemo` for a runnable save/load demo and
`mem/acceptancetests/checkpointresume` for a mid-transaction resume oracle.
