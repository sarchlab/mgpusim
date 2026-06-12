// Package latencyconn provides a connection with configurable latency.
package latencyconn

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim"
)

type ports struct {
	ports   []sim.Port
	portMap map[sim.RemotePort]int
}

func (p *ports) addPort(port sim.Port) {
	p.ports = append(p.ports, port)
	p.portMap[port.AsRemote()] = len(p.ports) - 1
}

func (p *ports) getPortIndex(index int) sim.Port {
	return p.ports[index]
}

func (p *ports) getPortByName(name sim.RemotePort) sim.Port {
	portIndex, found := p.portMap[name]
	if !found {
		panic(fmt.Sprintf("port %s not found", name))
	}

	return p.ports[portIndex]
}

func (p *ports) list() []sim.Port {
	return p.ports
}

func (p *ports) len() int {
	return len(p.ports)
}

type bufferedMsg struct {
	msg        sim.Msg
	cyclesLeft int
}

// Comp is a LatencyConnection that connects components with configurable
// latency. Messages are buffered for the specified number of cycles before
// delivery.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	ports             ports
	nextPortID        int
	latency           int
	crossGroupPenalty int
	portXCD           map[sim.RemotePort]int
	buffer            []bufferedMsg
}

// SetPortXCDGroup assigns an XCD group ID to the given port. Ports in
// different XCD groups incur the cross-group penalty on top of the base
// latency.
func (c *Comp) SetPortXCDGroup(port sim.Port, group int) {
	c.portXCD[port.AsRemote()] = group
}

// PlugIn marks the port as connected to this LatencyConnection.
func (c *Comp) PlugIn(port sim.Port) {
	c.Lock()
	defer c.Unlock()

	c.ports.addPort(port)

	port.SetConnection(c)
}

// Unplug marks the port as no longer connected to this LatencyConnection.
func (c *Comp) Unplug(_ sim.Port) {
	panic("not implemented")
}

// NotifyAvailable is called by a port to notify that the connection can
// deliver to the port again.
func (c *Comp) NotifyAvailable(p sim.Port) {
	for _, port := range c.ports.list() {
		if port == p {
			continue
		}

		port.NotifyAvailable()
	}

	c.TickNow()
}

// NotifySend is called by a port to notify that the connection can start
// to tick now.
func (c *Comp) NotifySend() {
	c.TickNow()
}

// Tick delegates to the middleware holder.
func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
}

// Tick updates the states of the connection, buffers new messages, and
// delivers messages whose latency has elapsed.
func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.deliverBuffered() || madeProgress

	for i := range m.ports.len() {
		portID := (i + m.nextPortID) % m.ports.len()
		port := m.ports.getPortIndex(portID)
		madeProgress = m.forwardMany(port) || madeProgress
	}

	m.nextPortID = (m.nextPortID + 1) % m.ports.len()

	// Keep ticking while buffer has pending messages.
	if len(m.buffer) > 0 {
		madeProgress = true
	}

	return madeProgress
}

func (m *middleware) forwardMany(port sim.Port) bool {
	madeProgress := false

	for {
		head := port.PeekOutgoing()
		if head == nil {
			break
		}

		effectiveLatency := m.latency

		if m.crossGroupPenalty > 0 {
			srcGroup, srcOK := m.portXCD[head.Meta().Src]
			dstGroup, dstOK := m.portXCD[head.Meta().Dst]

			if srcOK && dstOK && srcGroup != dstGroup {
				effectiveLatency += m.crossGroupPenalty
			}
		}

		m.buffer = append(m.buffer, bufferedMsg{
			msg:        head,
			cyclesLeft: effectiveLatency,
		})

		madeProgress = true

		port.RetrieveOutgoing()
	}

	return madeProgress
}

func (m *middleware) deliverBuffered() bool {
	madeProgress := false
	remaining := m.buffer[:0]

	for i := range m.buffer {
		m.buffer[i].cyclesLeft--

		if m.buffer[i].cyclesLeft <= 0 {
			dst := m.buffer[i].msg.Meta().Dst
			dstPort := m.ports.getPortByName(dst)

			err := dstPort.Deliver(m.buffer[i].msg)
			if err != nil {
				m.buffer[i].cyclesLeft = 0
				remaining = append(remaining, m.buffer[i])
				continue
			}

			madeProgress = true
		} else {
			remaining = append(remaining, m.buffer[i])
		}
	}

	m.buffer = remaining

	return madeProgress
}
