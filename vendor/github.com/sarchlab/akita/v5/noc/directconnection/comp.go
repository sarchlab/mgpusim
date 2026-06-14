// Package directconnection provides directconnection
package directconnection

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Spec holds immutable configuration for the DirectConnection.
type Spec struct {
	Freq timing.Freq `json:"freq"`
}

// State holds mutable runtime state for the DirectConnection.
type State struct {
	NextPortID int `json:"next_port_id"`
}

type ports struct {
	ports   []messaging.Port
	portMap map[messaging.RemotePort]int
}

func (p *ports) addPort(port messaging.Port) {
	p.ports = append(p.ports, port)
	p.portMap[port.AsRemote()] = len(p.ports) - 1
}

func (p *ports) getPortIndex(index int) messaging.Port {
	return p.ports[index]
}

func (p *ports) getPortByName(name messaging.RemotePort) messaging.Port {
	portIndex, found := p.portMap[name]
	if !found {
		panic(fmt.Sprintf("port %s not found", name))
	}
	return p.ports[portIndex]
}

func (p *ports) list() []messaging.Port {
	return p.ports
}

func (p *ports) len() int {
	return len(p.ports)
}

// Comp is a DirectConnection that connects components without latency.
type Comp struct {
	*modeling.Component[Spec, State, modeling.None]
}

func (c *Comp) mw() *middleware {
	return c.Middlewares()[0].(*middleware)
}

// PlugIn marks the port connects to this DirectConnection.
func (c *Comp) PlugIn(port messaging.Port) {
	c.Lock()
	defer c.Unlock()

	c.mw().ports.addPort(port)
	port.SetConnection(c)
}

// Unplug marks the port no longer connects to this DirectConnection.
func (c *Comp) Unplug(_ messaging.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify the connection can deliver again.
func (c *Comp) NotifyAvailable(p messaging.Port) {
	for _, port := range c.mw().ports.list() {
		if port == p {
			continue
		}
		port.NotifyAvailable()
	}
	c.TickNow()
}

// NotifySend is called by a port to notify the connection can start ticking.
func (c *Comp) NotifySend() {
	c.TickNow()
}

type middleware struct {
	comp  *modeling.Component[Spec, State, modeling.None]
	ports ports
}

func (m *middleware) Tick() bool {
	state := m.comp.State
	numPorts := m.ports.len()
	madeProgress := false

	for i := range numPorts {
		portID := (i + state.NextPortID) % numPorts
		port := m.ports.getPortIndex(portID)
		madeProgress = m.forwardMany(port) || madeProgress
	}

	(&m.comp.State).NextPortID = (state.NextPortID + 1) % numPorts

	return madeProgress
}

func (m *middleware) forwardMany(port messaging.Port) bool {
	madeProgress := false
	for {
		head := port.PeekOutgoing()
		if head == nil {
			break
		}
		dst := head.Meta().Dst
		dstPort := m.ports.getPortByName(dst)
		if !dstPort.CanDeliver() {
			break
		}

		dstPort.Deliver(head)
		madeProgress = true
		port.RetrieveOutgoing()
	}
	return madeProgress
}
