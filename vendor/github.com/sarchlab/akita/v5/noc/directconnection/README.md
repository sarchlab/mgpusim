# directconnection — Direct Point-to-Point Connection

Package `directconnection` provides a simple network connection for the Akita
simulation framework. It connects any number of ports and delivers messages
between them in a single tick, with no routing overhead or bandwidth modeling.

## How It Works

A `Comp` maintains a set of plugged-in ports. Each tick, it round-robins
through all ports, peeks at each port's outgoing buffer, resolves the
destination port by name, and delivers the message directly. Messages are
delivered in the same tick they are sent (subject to the connection's tick
frequency).

This makes `directconnection` ideal for connecting components that are
logically adjacent — for example, a cache and its memory controller, or
a compute unit and its L1 cache — without modeling a full network-on-chip.

## Key Types

### Comp

```go
type Comp struct {
    *sim.TickingComponent
    sim.MiddlewareHolder
}

func (c *Comp) PlugIn(port sim.Port)       // Connect a port
func (c *Comp) Unplug(port sim.Port)       // (not implemented)
func (c *Comp) NotifyAvailable(p sim.Port)  // Port buffer space freed
func (c *Comp) NotifySend()                 // Port has outgoing message
```

`Comp` implements `sim.Connection`, so ports can use it as their connection
for message delivery.

## Builder Pattern

```go
conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Connection")

conn.PlugIn(portA)
conn.PlugIn(portB)
conn.PlugIn(portC)
```

### Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithFreq(f)` | Tick frequency for the connection |

## Usage

```go
// Create engine and connection
engine := sim.NewSerialEngine()
conn := directconnection.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    Build("Bus")

// Create components with ports, then plug them in
conn.PlugIn(cache.GetPortByName("Bottom"))
conn.PlugIn(memCtrl.GetPortByName("Top"))
```

When component A sends a message to component B:
1. A places the message in its port's outgoing buffer with `Dst` set to B's
   port name.
2. The connection ticks, finds the message, resolves `Dst` to B's port, and
   calls `Deliver()`.
3. B receives the message in its port's incoming buffer on the next tick.

## Fairness

The connection uses round-robin scheduling across ports. The starting port
index advances by one each tick, ensuring no port is permanently starved.

## Limitations

- No bandwidth modeling — all pending messages are forwarded each tick.
- No latency modeling beyond the tick granularity.
- `Unplug` is not implemented.
- For simulations requiring realistic network modeling (latency, bandwidth,
  contention), use the `noc/networking` package instead.
