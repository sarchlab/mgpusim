package sim

import (
	"fmt"
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/naming"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
var HookPosPortMsgSend = &hooking.HookPos{Name: "Port Msg Send"}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port
var HookPosPortMsgRecvd = &hooking.HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieveIncoming marks when an inbound message is retrieved
// from the incoming buffer.
var HookPosPortMsgRetrieveIncoming = &hooking.HookPos{
	Name: "Port Msg Retrieve Incoming",
}

// HookPosPortMsgRetrieveOutgoing marks when an outbound message is retrieved
// from the outgoing buffer.
var HookPosPortMsgRetrieveOutgoing = &hooking.HookPos{
	Name: "Port Msg Retrieve Outgoing",
}

// A RemotePort is a string that refers to another port.
type RemotePort string

// A Port is owned by a component and is used to plugin connections
type Port interface {
	naming.Named
	hooking.Hookable

	AsRemote() RemotePort

	SetConnection(conn Connection)
	Component() Component
	SetComponent(comp Component)

	// For connection
	Deliver(msg Msg) *SendError
	NotifyAvailable()
	RetrieveOutgoing() Msg
	PeekOutgoing() Msg

	// For component
	CanSend() bool
	Send(msg Msg) *SendError
	RetrieveIncoming() Msg
	PeekIncoming() Msg

	// Buffer counts
	NumIncoming() int
	NumOutgoing() int

	// ForwardOutgoing iterates the outgoing buffer, calling tryDeliver for
	// each message. If tryDeliver returns true the message is removed;
	// otherwise it stays in place. Returns the number of messages forwarded.
	// This allows connections to skip blocked destinations without HOL
	// blocking on the head element.
	ForwardOutgoing(tryDeliver func(msg Msg) bool) int
}

// DefaultPort implements the port interface.
type defaultPort struct {
	hooking.HookableBase

	lock sync.Mutex
	name string
	comp Component
	conn Connection

	incomingBuf *portBuffer
	outgoingBuf *portBuffer
}

// AsRemote returns the remote port name.
func (p *defaultPort) AsRemote() RemotePort {
	return RemotePort(p.name)
}

// SetConnection sets which connection plugged in to this port.
func (p *defaultPort) SetConnection(conn Connection) {
	if p.conn != nil {
		connName := p.conn.Name()
		newConnName := conn.Name()
		panicMsg := fmt.Sprintf(
			"connection already set to %s, now connecting to %s",
			connName, newConnName,
		)
		panic(panicMsg)
	}

	p.conn = conn
}

// Component returns the owner component of the port.
func (p *defaultPort) Component() Component {
	return p.comp
}

// SetComponent sets the owner component of the port.
func (p *defaultPort) SetComponent(comp Component) {
	p.comp = comp
}

// Name returns the name of the port.
func (p *defaultPort) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *defaultPort) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	canSend := p.outgoingBuf.CanPush()

	return canSend
}

// Send is used to send a message out from a component
func (p *defaultPort) Send(msg Msg) *SendError {
	p.lock.Lock()

	p.msgMustBeValid(msg)

	if !p.outgoingBuf.CanPush() {
		p.lock.Unlock()
		return NewSendError()
	}

	p.outgoingBuf.Push(msg)

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)
	p.lock.Unlock()

	// Always notify the connection so it can forward the new message.
	// The connection's TickNow() is idempotent — if a tick is already
	// scheduled it will be a no-op. Without this, messages sent while
	// the outgoing buffer is non-empty can be stranded when the
	// connection's previous Tick() already forwarded earlier messages.
	p.conn.NotifySend()

	return nil
}

// Deliver is used to deliver a message to a component
func (p *defaultPort) Deliver(msg Msg) *SendError {
	p.lock.Lock()

	if !p.incomingBuf.CanPush() {
		p.lock.Unlock()
		return NewSendError()
	}

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.incomingBuf.Push(msg)
	p.lock.Unlock()

	// Always notify the owning component so it can process the new message.
	// The component's TickLater() is idempotent — if a tick is already
	// scheduled it will be a no-op.  Without this, messages delivered while
	// the buffer is non-empty can be stranded when the component's previous
	// Tick() returned false (no progress) and stopped scheduling itself.
	if p.comp != nil {
		p.comp.NotifyRecv(p)
	}

	return nil
}

// RetrieveIncoming is used by the component to take a message from the incoming
// buffer
func (p *defaultPort) RetrieveIncoming() Msg {
	p.lock.Lock()

	msg := p.incomingBuf.Pop()
	if msg == nil {
		p.lock.Unlock()
		return nil
	}

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.lock.Unlock()
		p.conn.NotifyAvailable(p)
	} else {
		p.lock.Unlock()
	}

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveIncoming,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg
}

// RetrieveOutgoing is used by the component to take a message from the outgoing
// buffer
func (p *defaultPort) RetrieveOutgoing() Msg {
	p.lock.Lock()

	msg := p.outgoingBuf.Pop()
	if msg == nil {
		p.lock.Unlock()
		return nil
	}

	if p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1 {
		p.comp.NotifyPortFree(p)
	}

	p.lock.Unlock()

	hookCtx := hooking.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieveOutgoing,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	return msg
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it.
func (p *defaultPort) PeekIncoming() Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Peek()
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it.
func (p *defaultPort) PeekOutgoing() Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Peek()
}

// NumIncoming returns the number of messages in the incoming buffer.
func (p *defaultPort) NumIncoming() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.incomingBuf.Size()
}

// NumOutgoing returns the number of messages in the outgoing buffer.
func (p *defaultPort) NumOutgoing() int {
	p.lock.Lock()
	defer p.lock.Unlock()

	return p.outgoingBuf.Size()
}

// ForwardOutgoing iterates the outgoing buffer, calling tryDeliver for
// each message. Messages for which tryDeliver returns true are removed;
// others remain in their original order. Returns the count of forwarded
// messages.
func (p *defaultPort) ForwardOutgoing(tryDeliver func(msg Msg) bool) int {
	p.lock.Lock()

	forwarded := 0
	wasFull := p.outgoingBuf.Size() >= p.outgoingBuf.Capacity()

	i := 0
	for i < p.outgoingBuf.Size() {
		msg := p.outgoingBuf.PeekAt(i)
		if msg == nil {
			break
		}

		// Temporarily unlock so that Deliver (which acquires the
		// destination port lock) doesn't deadlock with us.
		p.lock.Unlock()

		ok := tryDeliver(msg)

		p.lock.Lock()

		if ok {
			p.outgoingBuf.RemoveAt(i)
			forwarded++
			// Don't increment i — next element shifted into this slot.
		} else {
			i++
		}
	}

	needNotify := wasFull && p.outgoingBuf.Size() < p.outgoingBuf.Capacity()

	p.lock.Unlock()

	if needNotify && p.comp != nil {
		p.comp.NotifyPortFree(p)
	}

	// Fire hooks for forwarded messages.
	// (We skip individual hook calls here for simplicity; the
	// connection's Tick hook already covers the overall forwarding.)

	return forwarded
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *defaultPort) NotifyAvailable() {
	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}
}

// NewPort creates a new port with default behavior.
func NewPort(
	comp Component,
	incomingBufCap, outgoingBufCap int,
	name string,
) Port {
	p := new(defaultPort)
	p.comp = comp
	p.incomingBuf = newPortBuffer(incomingBufCap)
	p.outgoingBuf = newPortBuffer(outgoingBufCap)
	p.name = name

	return p
}

func (p *defaultPort) msgMustBeValid(msg Msg) {
	portMustBeMsgSrc(p, msg)
	dstMustNotBeEmpty(msg.Meta().Dst)
	srcDstMustNotBeTheSame(msg)
}

func portMustBeMsgSrc(port Port, msg Msg) {
	if port.Name() != string(msg.Meta().Src) {
		panic("sending port is not msg src")
	}
}

func dstMustNotBeEmpty(port RemotePort) {
	if port == "" {
		panic("dst is not given")
	}
}

func srcDstMustNotBeTheSame(msg Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
}
