package standalone

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
)

// TrafficMsg is a type of msg that only used in standalone network test.
// It has a byte size, but we do not care about the information it carries.
type TrafficMsg struct {
	sim.MsgMeta
}

// Meta returns the meta data of the message.
func (m *TrafficMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// NewTrafficMsg creates a new traffic message
func NewTrafficMsg(src, dst sim.Port, byteSize int) *TrafficMsg {
	msg := new(TrafficMsg)
	msg.Src = src
	msg.Dst = dst
	msg.TrafficBytes = byteSize
	return msg
}

// StartSendEvent is an event that triggers an agent to send a message.
type StartSendEvent struct {
	*sim.EventBase
	Msg *TrafficMsg
}

// NewStartSendEvent creates a new StartSendEvent.
func NewStartSendEvent(
	time sim.VTimeInSec,
	src, dst *Agent,
	byteSize int,
	trafficClass int,
) *StartSendEvent {
	e := new(StartSendEvent)
	e.EventBase = sim.NewEventBase(time, src)
	e.Msg = NewTrafficMsg(src.ToOut, dst.ToOut, byteSize)
	e.Msg.Meta().TrafficClass = trafficClass
	return e
}

// Agent is a component that connects the network. It can send and receive
// msg to/ from the network.
type Agent struct {
	*sim.TickingComponent

	ToOut sim.Port

	Buffer []*TrafficMsg
}

// NotifyRecv notifies that a port has received a message.
func (a *Agent) NotifyRecv(now sim.VTimeInSec, port sim.Port) {
	a.ToOut.Retrieve(now)
	a.TickLater(now)
}

// Handle defines how an agent handles events.
func (a *Agent) Handle(e sim.Event) error {
	switch e := e.(type) {
	case *StartSendEvent:
		a.handleStartSendEvent(e)
	case sim.TickEvent:
		a.TickingComponent.Handle(e)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (a *Agent) handleStartSendEvent(e *StartSendEvent) {
	a.Buffer = append(a.Buffer, e.Msg)
	a.TickLater(e.Time())
}

// Tick attempts to send a message out.
func (a *Agent) Tick(now sim.VTimeInSec) bool {
	return a.sendDataOut(now)
}

func (a *Agent) sendDataOut(now sim.VTimeInSec) bool {
	if len(a.Buffer) == 0 {
		return false
	}

	msg := a.Buffer[0]
	msg.Meta().SendTime = now
	err := a.ToOut.Send(msg)
	if err == nil {
		a.Buffer = a.Buffer[1:]
		return true
	}
	return false
}

// NewAgent creates a new agent.
func NewAgent(name string, engine sim.Engine) *Agent {
	a := new(Agent)
	a.TickingComponent = sim.NewTickingComponent(name, engine, 1*sim.GHz, a)

	a.ToOut = sim.NewLimitNumMsgPort(a, 4, name+".ToOut")

	return a
}
