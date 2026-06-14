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
    *modeling.Component[Spec, State, modeling.None]
}

func (c *Comp) PlugIn(port messaging.Port)          // Connect a port
func (c *Comp) Unplug(port messaging.Port)          // (not implemented)
func (c *Comp) NotifyAvailable(p messaging.Port)    // Port buffer space freed
func (c *Comp) NotifySend()                         // Port has outgoing message
```

`Comp` implements `messaging.Connection`, so ports can use it as their
connection for message delivery. The only configuration is the `Freq` field on
`Spec`, which sets the connection's tick frequency.

## Builder Pattern

A connection owns no resources, so it is configured by `Spec` alone and wired to
the simulation through a registrar. The registrar supplies the engine and
registers the connection.

```go
spec := directconnection.DefaultSpec()
spec.Freq = 1 * timing.GHz

conn := directconnection.MakeBuilder().
    WithRegistrar(reg).
    WithSpec(spec).
    Build("Connection")

conn.PlugIn(portA)
conn.PlugIn(portB)
conn.PlugIn(portC)
```

### Builder Methods

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and connection registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and set `Freq` |

## Usage

```go
// Create engine and connection
engine := timing.NewSerialEngine()
reg := modeling.NewStandaloneRegistrar(engine)
conn := directconnection.MakeBuilder().
    WithRegistrar(reg).
    WithSpec(directconnection.DefaultSpec()).
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
