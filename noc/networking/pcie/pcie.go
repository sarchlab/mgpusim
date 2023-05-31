// Package pcie provides a Connector and establishes a PCIe connection.
package pcie

import (
	"math"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/noc/networking/networkconnector"
	"github.com/sarchlab/mgpusim/v3/noc/networking/switching"
)

type switchNode struct {
	parentSwitchNode   *switchNode
	currSwitch         *switching.Switch
	localPortToParent  sim.Port
	remotePortToParent sim.Port
}

// Connector can connect devices into a PCIe network.
type Connector struct {
	freq             sim.Freq
	bandwidth        uint64
	encodingOverhead float64
	flitByteSize     int
	switchLatency    int
	connector        networkconnector.Connector
}

// NewConnector creates a new connector that can help configure PCIe networks.
func NewConnector() *Connector {
	c := &Connector{}

	c.connector = networkconnector.MakeConnector()

	c = c.WithFrequency(1*sim.GHz).
		WithVersion(4, 16).
		WithSwitchLatency(140)

	return c
}

// WithEngine sets the event-driven simulation engine that the PCIe connection
// uses.
func (c *Connector) WithEngine(engine sim.Engine) *Connector {
	c.connector = c.connector.WithEngine(engine)
	return c
}

// WithFrequency sets the frequency used by the components in the connection. It
// does not have to be the exact frequency of the network. Instead, it is better
// to set as same frequency that the network interfaces work at.
func (c *Connector) WithFrequency(freq sim.Freq) *Connector {
	c.connector = c.connector.WithDefaultFreq(freq)
	c.freq = freq
	return c
}

// WithMonitor sets the monitor that inspects the states of the component
// associated with the connection.
func (c *Connector) WithMonitor(m *monitoring.Monitor) *Connector {
	c.connector = c.connector.WithMonitor(m)
	return c
}

// WithBandwidth sets the bandwidth of all the connections in the PCIe network.
func (c *Connector) WithBandwidth(bytePerSecond uint64) *Connector {
	c.bandwidth = bytePerSecond
	c.flitByteSize = int(math.Round(float64(c.bandwidth) / float64(c.freq)))

	if c.flitByteSize == 0 {
		panic("flit size is 0")
	}

	return c
}

// WithVersion sets the version of the network.
func (c *Connector) WithVersion(version int, width int) *Connector {
	linkBandwidthTable := map[int]uint64{
		1: 2 * (1 << 30),
		2: 4 * (1 << 30),
		3: 8 * (1 << 30),
		4: 16 * (1 << 30),
		5: 32 * (1 << 30),
	}

	linkBandwidth := linkBandwidthTable[version]
	totalBandwidth := linkBandwidth * uint64(width)

	return c.WithBandwidth(totalBandwidth)
}

// WithSwitchLatency sets the extra latency on each switch before a switch
// can forward a flit.
func (c *Connector) WithSwitchLatency(numCycles int) *Connector {
	c.switchLatency = numCycles
	return c
}

// WithVisTracer sets the vis tracer that can be used to visualize the network.
func (c *Connector) WithVisTracer(tracer tracing.Tracer) *Connector {
	c.connector = c.connector.WithVisTracer(tracer)
	return c
}

// CreateNetwork creates a network. This function should be called before
// creating root complexes.
func (c *Connector) CreateNetwork(name string) {
	c.connector.NewNetwork(name)
}

// AddRootComplex adds a new switch connecting CPU ports.
func (c *Connector) AddRootComplex(cpuPorts []sim.Port) (switchID int) {
	switchID = c.connector.AddSwitch()

	c.PlugInDevice(switchID, cpuPorts)

	return switchID
}

// AddSwitch adds a new switch connecting from an existing switch.
func (c *Connector) AddSwitch(baseSwitchID int) (switchID int) {
	switchID = c.connector.AddSwitch()

	c.connector.ConnectSwitches(baseSwitchID, switchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.switchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.switchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:   true,
				Frequency: c.freq,
			},
		})

	return switchID
}

// PlugInDevice connects a series of ports to a switch.
func (c *Connector) PlugInDevice(baseSwitchID int, devicePorts []sim.Port) {
	c.connector.ConnectDevice(baseSwitchID, devicePorts,
		networkconnector.DeviceToSwitchLinkParameter{
			DeviceEndParam: networkconnector.LinkEndDeviceParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			SwitchEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.switchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:   true,
				Frequency: c.freq,
			},
		})
}

// EstablishRoute populates the routing tables in the network.
func (c *Connector) EstablishRoute() {
	c.connector.EstablishRoute()
}
