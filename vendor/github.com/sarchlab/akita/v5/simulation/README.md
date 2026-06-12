# simulation

Package `simulation` provides the top-level simulation runner that wires
together an engine, data recorder, visual tracer, and optional monitoring
server. It also provides checkpoint save/load support.

## Simulation

A `Simulation` holds:

- An `Engine` (serial or parallel) for event processing.
- A `DataRecorder` (SQLite-backed) for recording simulation data.
- A `DBTracer` for visual tracing (used by the Daisen visualizer).
- An optional `Monitor` for live web-based inspection.
- A registry of all components and ports.

### Building a Simulation

```go
sim := simulation.MakeBuilder().
    WithParallelEngine().       // optional: use parallel engine
    WithMonitorPort(8080).      // optional: monitoring server port
    WithVisTracingOnStart().    // optional: start tracing immediately
    Build()

defer sim.Terminate()
```

Builder options:

| Method | Description |
|--------|-------------|
| `WithParallelEngine()` | Use `ParallelEngine` instead of `SerialEngine` |
| `WithoutMonitoring()` | Disable the monitoring web server |
| `WithMonitorPort(port)` | Set the monitoring server port |
| `WithOutputFileName(name)` | Custom SQLite output file name |
| `WithVisTracingOnStart()` | Enable visual tracing from time 0 |

### Registering Components

```go
sim.RegisterComponent(myComponent)
```

Registration automatically:
- Adds the component and its ports to the simulation registry.
- Connects the component to the monitoring system (if enabled).
- Attaches visual tracing hooks.

### Accessing Simulation Resources

```go
engine := sim.GetEngine()
recorder := sim.GetDataRecorder()
tracer := sim.GetVisTracer()
comp := sim.GetComponentByName("myComp")
port := sim.GetPortByName("myComp.Top")
```

## Checkpoint Save / Load

Save and load full simulation state to/from a directory:

```go
// Save (requires quiescence — all port buffers must be empty)
err := sim.Save("/path/to/checkpoint")

// Load (simulation must already be built with same topology)
err := sim.Load("/path/to/checkpoint")
```

### What Gets Saved

- **Metadata**: engine time, ID generator state.
- **Component states**: JSON files for each component implementing `StateSaver`.
- **Storage data**: binary files for each component implementing
  `StorageOwner`.

### What Is NOT Saved

- Event queues (reconstructed via `TickLater` / port notifications).
- Connections (reconstructed from build code).
- Port buffer contents (must be empty — quiescence required).

### Interfaces for Checkpointing

| Interface | Method | Purpose |
|-----------|--------|---------|
| `StateSaver` | `SaveState(w io.Writer)` | Serialize component state |
| `StateLoader` | `LoadState(r io.Reader)` | Deserialize component state |
| `StorageOwner` | `GetStorage() *mem.Storage` | Access storage for binary save |
| `TickResetter` | `ResetTick()` | Reset tick scheduler after load |
| `WakeupResetter` | `ResetWakeup()` | Reset wakeup guard after load |

`modeling.Component[S,T]` and `modeling.EventDrivenComponent[S,T]` satisfy
these interfaces automatically.
