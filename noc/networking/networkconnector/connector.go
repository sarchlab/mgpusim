package networkconnector

import (
	"fmt"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/sim/bottleneckanalysis"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/noc/messaging"
	"github.com/sarchlab/mgpusim/v3/noc/networking/arbitration"
	"github.com/sarchlab/mgpusim/v3/noc/networking/routing"
	"github.com/sarchlab/mgpusim/v3/noc/networking/switching"
)

// LinkEndSwitchParameter defines the parameter that associated with an end of a
// link that is connected to a switch.
type LinkEndSwitchParameter struct {
	IncomingBufSize  int
	OutgoingBufSize  int
	NumInputChannel  int
	NumOutputChannel int
	Latency          int
	PortName         string
}

// LinkEndDeviceParameter defines the parameter that associated with an end of a
// link that is connected to a device.
type LinkEndDeviceParameter struct {
	IncomingBufSize  int
	OutgoingBufSize  int
	NumInputChannel  int
	NumOutputChannel int
}

// LinkParameter defines the parameter of the link that connects to nodes.
type LinkParameter struct {
	IsIdeal       bool
	Frequency     sim.Freq
	NumStage      int
	CyclePerStage int
	PipelineWidth int
}

// DeviceToSwitchLinkParameter contains the parameters that define a link
// between a device and a switch.
type DeviceToSwitchLinkParameter struct {
	DeviceEndParam LinkEndDeviceParameter
	SwitchEndParam LinkEndSwitchParameter
	LinkParam      LinkParameter
}

// SwitchToSwitchLinkParameter contains the parameters that define a link
// between two switches.
type SwitchToSwitchLinkParameter struct {
	LeftEndParam  LinkEndSwitchParameter
	RightEndParam LinkEndSwitchParameter
	LinkParam     LinkParameter
}

// Connector can build complex network topologies.
type Connector struct {
	name           string
	engine         sim.Engine
	monitor        *monitoring.Monitor
	defaultFreq    sim.Freq
	flitSize       int
	router         Router
	visTracer      tracing.Tracer
	nocTracer      tracing.Tracer
	bufferAnalyzer *bottleneckanalysis.BufferAnalyzer

	switches        []*switchNode
	devices         []*deviceNode
	connectionCount int
}

// MakeConnector creates a network connector
func MakeConnector() Connector {
	return Connector{
		defaultFreq: 1 * sim.GHz,
		flitSize:    64,
		router:      new(FloydWarshallRouter),
	}
}

// WithEngine sets the engine to be used by all the components in the
// connection.
func (c Connector) WithEngine(e sim.Engine) Connector {
	c.engine = e
	return c
}

// WithMonitor sets the monitor that monitors all the components in the
// connection.
func (c Connector) WithMonitor(m *monitoring.Monitor) Connector {
	c.monitor = m
	return c
}

// WithDefaultFreq sets the default frequency used by the components in the
// connection. Note that channels will not use the default frequency. Channels
// use their own frequency to adjust bandwidth.
func (c Connector) WithDefaultFreq(f sim.Freq) Connector {
	c.defaultFreq = f
	return c
}

// WithFlitSize sets the flit size to be used throughout the network.
func (c Connector) WithFlitSize(size int) Connector {
	c.flitSize = size
	return c
}

// WithRouter sets the router to use to establish the routing tables.
func (c Connector) WithRouter(r Router) Connector {
	c.router = r
	return c
}

// WithVisTracer sets the tracer used to trace tasks in the network.
func (c Connector) WithVisTracer(t tracing.Tracer) Connector {
	c.visTracer = t
	return c
}

// WithNoCTracer sets the tracer used to trace NoC-specific metrics, such as the
// traffics and congestions in the channels.
func (c Connector) WithNoCTracer(t tracing.Tracer) Connector {
	c.nocTracer = t
	return c
}

// WithBufferAnalyzer sets the buffer analyzer that can record the buffer levels in the network.
func (c Connector) WithBufferAnalyzer(
	b *bottleneckanalysis.BufferAnalyzer,
) Connector {
	c.bufferAnalyzer = b
	return c
}

// GetFlitSize returns the flit size used by the network.
func (c *Connector) GetFlitSize() int {
	return c.flitSize
}

// NewNetwork resets the connector, making it ready to create a new network
// with the give Name.
func (c *Connector) NewNetwork(name string) {
	c.name = name
	c.switches = nil
}

// AddSwitch adds a new switch to the network.
func (c *Connector) AddSwitch() (switchID int) {
	switchID = len(c.switches)
	name := fmt.Sprintf("Switch[%d]", switchID)

	c.AddSwitchWithName(name)

	return switchID
}

// AddSwitchWithNameAndRoutingTable adds a new switch to the network with an
// externally provided name and routing table.
func (c *Connector) AddSwitchWithNameAndRoutingTable(
	swName string,
	rt routing.Table,
) (switchID int) {
	switchID = len(c.switches)
	arbiter := arbitration.NewXBarArbiter()

	name := fmt.Sprintf("%s.%s", c.name, swName)
	sw := switching.SwitchBuilder{}.
		WithEngine(c.engine).
		WithFreq(c.defaultFreq).
		WithArbiter(arbiter).
		WithRoutingTable(rt).
		Build(name)

	if c.monitor != nil {
		c.monitor.RegisterComponent(sw)
	}

	if c.visTracer != nil {
		tracing.CollectTrace(sw, c.visTracer)
	}

	if c.bufferAnalyzer != nil {
		c.bufferAnalyzer.AddComponent(sw)
	}

	node := &switchNode{
		sw: sw,
	}

	c.switches = append(c.switches, node)

	return switchID
}

// AddSwitchWithName adds a new switch to the network with an externally
// provided Name.
func (c *Connector) AddSwitchWithName(swName string) (switchID int) {
	routingTable := routing.NewTable()
	return c.AddSwitchWithNameAndRoutingTable(swName, routingTable)
}

type namedHookableConnection interface {
	sim.Connection
	sim.Named
	sim.Hookable
	sim.Component
}

// ConnectDevice connects a few ports that belongs to the device to a switch
// that is identified by switchID.
func (c *Connector) ConnectDevice(
	switchID int,
	ports []sim.Port,
	param DeviceToSwitchLinkParameter,
) {
	swNode := c.switches[switchID]

	epNode := c.createEndPoint(ports, param, swNode)
	swPort, conn := c.connectEndPointWithSwitch(swNode, epNode.endPoint, param)
	c.createRemoteInfoFoEP(epNode, swNode, epNode.endPoint.NetworkPort, swPort, conn)
}

// ConnectDeviceWithEPName connects a few ports that belongs to the device to a
// switch that is identified by switchID.
func (c *Connector) ConnectDeviceWithEPName(
	epName string,
	switchID int,
	ports []sim.Port,
	param DeviceToSwitchLinkParameter,
) (epPort, swPort sim.Port) {
	swNode := c.switches[switchID]

	epNode := c.createEndPointWithName(ports, param, swNode, epName)
	swPort, conn := c.connectEndPointWithSwitch(swNode, epNode.endPoint, param)
	c.createRemoteInfoFoEP(
		epNode, swNode, epNode.endPoint.NetworkPort, swPort,
		conn)

	return epPort, swPort
}

func (c *Connector) createEndPointWithName(
	ports []sim.Port,
	param DeviceToSwitchLinkParameter,
	swNode *switchNode,
	name string,
) *deviceNode {
	fullName := fmt.Sprintf("%s.%s", c.name, name)
	endPoint := switching.MakeEndPointBuilder().
		WithEngine(c.engine).
		WithFreq(c.defaultFreq).
		WithFlitByteSize(c.flitSize).
		WithDevicePorts(ports).
		WithNumInputChannels(param.DeviceEndParam.NumInputChannel).
		WithNumOutputChannels(param.DeviceEndParam.NumOutputChannel).
		Build(fullName)

	if c.monitor != nil {
		c.monitor.RegisterComponent(endPoint)
	}

	if c.visTracer != nil {
		tracing.CollectTrace(endPoint, c.visTracer)
	}

	epPort := sim.NewLimitNumMsgPort(endPoint,
		param.DeviceEndParam.IncomingBufSize,
		endPoint.Name()+".NetworkPort")
	endPoint.NetworkPort = epPort

	epNode := &deviceNode{
		ports:    ports,
		endPoint: endPoint,
		sw:       swNode,
	}
	c.devices = append(c.devices, epNode)

	return epNode
}

func (c *Connector) createEndPoint(
	ports []sim.Port,
	param DeviceToSwitchLinkParameter,
	swNode *switchNode,
) *deviceNode {
	name := fmt.Sprintf("EndPoint[%d]", len(c.devices))
	return c.createEndPointWithName(ports, param, swNode, name)
}

func (c *Connector) connectEndPointWithSwitch(
	swNode *switchNode, endPoint *switching.EndPoint,
	param DeviceToSwitchLinkParameter,
) (*sim.LimitNumMsgPort, namedHookableConnection) {
	sw := swNode.sw
	epPort := endPoint.NetworkPort

	swPort := sim.NewLimitNumMsgPort(sw,
		param.SwitchEndParam.IncomingBufSize,
		fmt.Sprintf("%s.Port[%d]", sw.Name(), len(swNode.remotes)))
	endPoint.DefaultSwitchDst = swPort
	switching.MakeSwitchPortAdder(sw).
		WithPorts(swPort, epPort).
		WithLatency(param.SwitchEndParam.Latency).
		WithNumInputChannel(param.SwitchEndParam.NumInputChannel).
		WithNumOutputChannel(param.SwitchEndParam.NumOutputChannel).
		AddPort()

	conn := c.connectPorts(epPort, swPort,
		param.DeviceEndParam.OutgoingBufSize,
		param.SwitchEndParam.OutgoingBufSize,
		param.LinkParam,
	)

	return swPort, conn
}

func (c *Connector) createRemoteInfoFoEP(
	epNode *deviceNode, swNode *switchNode,
	epPort, swPort sim.Port,
	conn namedHookableConnection,
) {
	epNode.remote = Remote{
		LocalNode:  epNode,
		LocalPort:  epPort,
		RemoteNode: swNode,
		RemotePort: swPort,
		Link:       conn,
	}
	swNode.remotes = append(swNode.remotes, Remote{
		LocalNode:  swNode,
		LocalPort:  swPort,
		RemoteNode: epNode,
		RemotePort: epPort,
		Link:       conn,
	})
}

func (c *Connector) connectPorts(
	left, right sim.Port,
	leftBufSize, rightBufSize int,
	linkParam LinkParameter,
) (conn namedHookableConnection) {
	connName := fmt.Sprintf("%s.Conn[%d]", c.name, c.connectionCount)
	c.connectionCount++

	if linkParam.IsIdeal {
		conn = sim.NewDirectConnection(connName, c.engine, c.defaultFreq)
	} else {
		conn = messaging.MakeChannelBuilder().
			WithEngine(c.engine).
			WithPipelineParameters(
				linkParam.NumStage,
				linkParam.CyclePerStage,
				linkParam.PipelineWidth).
			WithFreq(linkParam.Frequency).
			Build(connName)
	}
	conn.PlugIn(left, leftBufSize)
	conn.PlugIn(right, rightBufSize)

	if c.monitor != nil {
		c.monitor.RegisterComponent(conn)
	}

	if c.visTracer != nil {
		tracing.CollectTrace(conn.(tracing.NamedHookable), c.visTracer)
	}

	if c.nocTracer != nil {
		tracing.CollectTrace(conn.(tracing.NamedHookable), c.nocTracer)
	}

	if c.bufferAnalyzer != nil {
		c.bufferAnalyzer.AddPort(left)
		c.bufferAnalyzer.AddPort(right)
	}

	return conn
}

// ConnectSwitches create a connection between two switches. The connection
// created is bi-directional.
func (c *Connector) ConnectSwitches(
	leftSwitchID, rightSwitchID int,
	param SwitchToSwitchLinkParameter,
) (leftPort, rightPort sim.Port) {
	leftNode := c.switches[leftSwitchID]
	leftSwitch := leftNode.sw
	leftPortName := leftSwitch.Name() + "." + param.LeftEndParam.PortName
	if param.LeftEndParam.PortName == "" {
		leftPortName = fmt.Sprintf("%s.Port%d",
			leftSwitch.Name(), len(leftNode.remotes))
	}
	leftPort = sim.NewLimitNumMsgPort(leftSwitch,
		param.LeftEndParam.IncomingBufSize,
		leftPortName)

	rightNode := c.switches[rightSwitchID]
	rightSwitch := rightNode.sw
	rightPortName := rightSwitch.Name() + "." + param.RightEndParam.PortName
	if param.RightEndParam.PortName == "" {
		rightPortName = fmt.Sprintf("%s.Port%d",
			rightSwitch.Name(), len(rightNode.remotes))
	}
	rightPort = sim.NewLimitNumMsgPort(rightSwitch,
		param.RightEndParam.IncomingBufSize, rightPortName)

	switching.MakeSwitchPortAdder(leftSwitch).
		WithPorts(leftPort, rightPort).
		WithLatency(param.LeftEndParam.Latency).
		WithNumInputChannel(param.LeftEndParam.NumInputChannel).
		WithNumOutputChannel(param.LeftEndParam.NumOutputChannel).
		AddPort()

	switching.MakeSwitchPortAdder(rightSwitch).
		WithPorts(rightPort, leftPort).
		WithLatency(param.RightEndParam.Latency).
		WithNumInputChannel(param.RightEndParam.NumInputChannel).
		WithNumOutputChannel(param.RightEndParam.NumOutputChannel).
		AddPort()

	conn := c.connectPorts(leftPort, rightPort,
		param.LeftEndParam.OutgoingBufSize,
		param.RightEndParam.OutgoingBufSize,
		param.LinkParam)

	c.createRemoteInfo(leftNode, rightNode, leftPort, rightPort, conn)

	return leftPort, rightPort
}

func (c *Connector) createRemoteInfo(
	leftNode, rightNode *switchNode,
	leftPort, rightPort sim.Port,
	conn namedHookableConnection,
) {
	leftNode.remotes = append(leftNode.remotes, Remote{
		LocalNode:  leftNode,
		LocalPort:  leftPort,
		RemoteNode: rightNode,
		RemotePort: rightPort,
		Link:       conn,
	})
	rightNode.remotes = append(rightNode.remotes, Remote{
		LocalNode:  rightNode,
		LocalPort:  rightPort,
		RemoteNode: leftNode,
		RemotePort: leftPort,
		Link:       conn,
	})
}

// EstablishRoute sets the routing table for all the nodes.
func (c *Connector) EstablishRoute() {
	if c.router == nil {
		return
	}

	nodes := c.createRoutingNodeList()
	c.router.EstablishRoute(nodes)
	// c.dumpRoute()
}

func (c *Connector) createRoutingNodeList() []Node {
	nodes := make([]Node, 0, len(c.devices)+len(c.switches))

	for _, d := range c.devices {
		nodes = append(nodes, d)
	}

	for _, s := range c.switches {
		nodes = append(nodes, s)
	}

	return nodes
}

func (c *Connector) dumpRoute() {
	fmt.Println("")
	for _, swNode := range c.switches {
		for _, epNode := range c.devices {
			for _, port := range epNode.ports {
				nextHopPort := swNode.sw.GetRoutingTable().FindPort(port)

				var nextHop Node
				for _, remote := range swNode.remotes {
					if remote.LocalPort == nextHopPort {
						nextHop = remote.RemoteNode
					}
				}

				fmt.Printf("%s -> %s -> %s --- %s\n",
					swNode.Name(),
					nextHopPort.Name(),
					nextHop.Name(),
					port.Name())
			}
		}
	}
}
