# messaging — Messages, Ports, and Connections

Package `messaging` provides messages, ports, and connections for the Akita
simulation framework. It is the communication layer: components own ports,
ports buffer messages, and a connection moves messages from one port's outgoing
buffer to another port's incoming buffer.

## Key Concepts

- A **message** (`Msg`) is any value carrying a `*MsgMeta` with routing and
  identification metadata. Bare `MsgMeta` is the envelope, not a message — it
  belongs to no protocol.
- A **protocol** (`Protocol`) is a named set of message types organized into
  **roles** (`Role`). Defining a protocol with `DefineProtocol` registers
  every message type it carries with the checkpoint codec; ports declare the
  role(s) they speak in `DeclarePort`. Protocols are **opt-in**: messages
  flow without one, and registration only matters when a checkpoint can
  capture the message.
- A **port** is owned by a component and holds an incoming and an outgoing
  buffer. Components `Send`/`RetrieveIncoming` on their side; connections
  `Deliver`/`RetrieveOutgoing` on theirs.
- A **connection** is plugged into a set of ports and is responsible for
  delivering each outgoing message to the destination port named in its
  metadata.
- Ports are addressed by `RemotePort`, a string name. A message's `Src` and
  `Dst` are remote port names, not pointers.

## Key Types

### Msg and MsgMeta

```go
type Msg interface {
    Meta() *MsgMeta
}

type MsgMeta struct {
    ID           uint64
    Src, Dst     RemotePort
    TrafficClass string
    TrafficBytes int
    RspTo        uint64 // ID of the request this responds to, if any
    SendTaskID   uint64
    RecvTaskID   uint64
}
```

Embed `MsgMeta` (or hold one) so a message satisfies `Msg`. `meta.IsRsp()`
reports whether `RspTo` is set.

**Checkpointing:** a message buffered in a port is serialized when the simulation
is checkpointed, so each concrete message type must be registered. The
recommended way is to declare the package's protocol once:

```go
var (
    Protocol  = messaging.DefineProtocol("mem",
        messaging.RoleDef{Name: "requester",
            Sends: []messaging.Msg{ReadReq{}, WriteReq{}}},
        messaging.RoleDef{Name: "responder",
            Sends: []messaging.Msg{DataReadyRsp{}, WriteDoneRsp{}}},
    )
    Requester = Protocol.Role("requester")
    Responder = Protocol.Role("responder")
)
```

and bind ports to roles where they are declared:
`comp.DeclarePort("Top", memprotocol.Responder)`. Each protocol lives in its
own package (e.g. `mem/memprotocol`, `mem/memcontrolprotocol`, `mem/vm/vmprotocol`)
that owns the message types and the protocol definition. A
registration-coverage audit
(`protocolaudit_test.go`) fails CI for any message type in the module that is
not registered. `RegisterMsg(MyReq{})` in an `init()` remains as the low-level
primitive (and `RegisterEvent` for events). No custom marshalling is needed.
See [`doc/tutorial/checkpointing.md`](../doc/tutorial/checkpointing.md).

### Port

```go
type Port interface {
    naming.Named
    hooking.Hookable

    AsRemote() RemotePort

    // For the component side
    CanSend() bool
    Send(msg Msg)
    PeekIncoming() Msg
    RetrieveIncoming() Msg

    // For the connection side
    Deliver(msg Msg)
    PeekOutgoing() Msg
    RetrieveOutgoing() Msg
    NotifyAvailable()

    SetConnection(conn Connection)
    Component() Component
    SetComponent(comp Component)
    NumIncoming() int
    NumOutgoing() int
}
```

Create a port with `NewPort`:

```go
port := messaging.NewPort(comp, incomingCap, outgoingCap, "MyComp.Top")
```

In assembly, prefer `modeling.MakePortBuilder` — it wraps `NewPort` and
registers the port with the simulation (and the monitor) through the registrar,
mirroring how component and connection builders register themselves. `NewPort`
is the low-level constructor it builds on.

`Send` pushes onto the outgoing buffer — callers must check `CanSend` first;
sending into a full buffer panics — and, when the buffer transitions from
empty, notifies the connection via `NotifySend`. `Deliver` pushes onto the
incoming buffer (guarded by `CanDeliver` the same way) and notifies the owning
component via `NotifyRecv`.

### Connection

```go
type Connection interface {
    naming.Named
    hooking.Hookable

    PlugIn(port Port)
    Unplug(port Port)
    NotifyAvailable(port Port)
    NotifySend()
}
```

A connection moves messages from outgoing to incoming buffers. `directconnection`
is the simplest implementation.

### Component and PortOwner

```go
type Component interface {
    naming.Named
    hooking.Hookable
    PortOwner

    NotifyRecv(port Port)
    NotifyPortFree(port Port)
}

type PortOwner interface {
    DeclarePort(name string, roles ...*Role)
    AssignPort(name string, port Port)
    GetPortByName(name string) Port
    Ports() []Port
}
```

Embed `PortOwnerBase` (via `NewPortOwnerBase`) to manage a named set of ports.
A component owns its port topology: it declares its ports with `DeclarePort`
(typically in its builder), and setup code supplies the instances with
`AssignPort`. `GetPortByName` panics with a helpful message if the name is
unknown or was declared but not yet assigned.

## How It Works

1. A component declares its ports with `DeclarePort`; setup code builds each
   port with `modeling.MakePortBuilder` (which registers it with the
   simulation) — or the low-level `NewPort` — and attaches it with `AssignPort`.
2. A connection is plugged into the ports with `PlugIn`, and each port's
   connection is set with `SetConnection`.
3. To send, a component builds a message with `Src`/`Dst` remote port names and
   calls `port.Send(msg)`.
4. The connection picks up the message via `PeekOutgoing`/`RetrieveOutgoing`,
   resolves `Dst`, and calls the destination port's `Deliver`.
5. The destination component is notified by `NotifyRecv` and consumes the
   message with `PeekIncoming`/`RetrieveIncoming`.

## Hooks

Ports are `hooking.Hookable` and fire hooks (with the message as `HookCtx.Item`)
at `HookPosPortMsgSend`, `HookPosPortMsgRecvd`, `HookPosPortMsgRetrieveIncoming`,
and `HookPosPortMsgRetrieveOutgoing`, which tracing and instrumentation use to
observe traffic.
