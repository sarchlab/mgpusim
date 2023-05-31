// Package nvlink provides a connector that can create a network that includes
// PCIe, NVLink, and ethernet network.
package nvlink

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/noc/networking/networkconnector"
	"github.com/sarchlab/mgpusim/v3/noc/networking/switching"
)

// A deviceNode represents a switch associated with the device and
// and NVLink switch.
type deviceNode struct {
	endpoint       *switching.EndPoint
	deviceSwitchID int
	nvlinkSwitchID int
}

// Connector can connect devices into a network that includes PCIe, NVLink,
// and ethernet network.
type Connector struct {
	networkName      string
	freq             sim.Freq
	encodingOverhead float64
	flitByteSize     int

	pcieBandwidth     uint64
	pcieSwitchLatency int

	nvlinkBandwidth     uint64
	nvlinkSwitchLatency int

	ethernetSwitchLatency int
	ethernetBandwidth     uint64

	connector networkconnector.Connector

	devices          []*deviceNode
	pcieSwitches     map[int]bool
	ethernetSwitches map[int]bool
}

// NewConnector creates a new connector that can help configure PCIe networks.
func NewConnector() *Connector {
	c := &Connector{}

	c.connector = networkconnector.MakeConnector()

	c.ethernetSwitches = make(map[int]bool)
	c.pcieSwitches = make(map[int]bool)

	c = c.WithFrequency(1*sim.GHz).
		WithPCIeVersion(4, 16).
		WithPCIeSwitchLatency(140).
		WithNVLinkVersion(2).
		WithNVLinkSwitchLatency(140).
		WithEthernetSwitchLatency(100000).
		WithEthernetBandwidth(1.25 * (1 << 30))

	c.connector = c.connector.WithRouter(&networkconnector.BandwidthFirstRouter{
		FlitSize: c.flitByteSize,
	})

	return c
}

// WithMonitor sets the monitor that monitors the components in the connection.
func (c *Connector) WithMonitor(m *monitoring.Monitor) *Connector {
	c.connector = c.connector.WithMonitor(m)
	return c
}

// WithEngine sets the event-driven simulation engine that the PCIe connection
// uses.
func (c *Connector) WithEngine(engine sim.Engine) *Connector {
	c.connector = c.connector.WithEngine(engine)
	return c
}

// WithFrequency sets the frequency of the components in the connection.
func (c *Connector) WithFrequency(freq sim.Freq) *Connector {
	c.freq = freq
	c.connector = c.connector.WithDefaultFreq(freq)
	return c
}

// WithPCIeBandwidth sets the bandwidth of all the connections in the PCIe
// network.
func (c *Connector) WithPCIeBandwidth(bytePerSecond uint64) *Connector {
	c.pcieBandwidth = bytePerSecond
	c.flitByteSize = int(math.Round(float64(c.pcieBandwidth) / float64(c.freq)))

	if c.flitByteSize == 0 {
		panic("flit size is 0")
	}

	c.connector = c.connector.
		WithFlitSize(c.flitByteSize).
		WithRouter(&networkconnector.BandwidthFirstRouter{
			FlitSize: c.flitByteSize,
		})

	return c
}

// WithPCIeVersion sets the version of the PCIe network.
func (c *Connector) WithPCIeVersion(version int, width int) *Connector {
	transferPerSecondTable := map[int]uint64{
		1: 2 * (1 << 30),
		2: 4 * (1 << 30),
		3: 8 * (1 << 30),
		4: 16 * (1 << 30),
		5: 32 * (1 << 30),
	}

	transferPerSecond := transferPerSecondTable[version]
	totalBandwidth := transferPerSecond * uint64(width) / 8

	return c.WithPCIeBandwidth(totalBandwidth)
}

// WithPCIeSwitchLatency sets the latency of each switch before the data can be
// forwarded to the next hop.
func (c *Connector) WithPCIeSwitchLatency(numCycles int) *Connector {
	c.pcieSwitchLatency = numCycles
	return c
}

// WithNVLinkBandwidth sets the bandwidth of each bandwidth link.
func (c *Connector) WithNVLinkBandwidth(bytePerSecond uint64) *Connector {
	c.nvlinkBandwidth = bytePerSecond
	return c
}

// WithNVLinkVersion sets the version of the NVLink to use.
func (c *Connector) WithNVLinkVersion(version int) *Connector {
	switch version {
	case 1:
		c.nvlinkBandwidth = 2.5 * (1 << 30) * 8
	case 2:
		c.nvlinkBandwidth = 3.125 * (1 << 30) * 8
	case 3:
		c.nvlinkBandwidth = 6.25 * (1 << 30) * 8
	default:
		panic("NVLink version not supported.")
	}

	return c
}

// WithNVLinkSwitchLatency sets the latency of each NVLink switch.
func (c *Connector) WithNVLinkSwitchLatency(numCycle int) *Connector {
	c.nvlinkSwitchLatency = numCycle

	return c
}

// WithEthernetSwitchLatency sets the latency of the ethernet switch.
func (c *Connector) WithEthernetSwitchLatency(numCycle int) *Connector {
	c.ethernetSwitchLatency = numCycle
	return c
}

// WithEthernetBandwidth sets the bandwidth of each ethernet link.
func (c *Connector) WithEthernetBandwidth(bytePerSecond uint64) *Connector {
	c.ethernetBandwidth = bytePerSecond
	return c
}

// WithVisTracer sets the tracer used to trace tasks in the network.
func (c Connector) WithVisTracer(t tracing.Tracer) Connector {
	c.connector = c.connector.WithVisTracer(t)
	return c
}

// CreateNetwork creates a network. This function should be called before
// creating root complexes.
func (c *Connector) CreateNetwork(name string) {
	c.connector.NewNetwork(name)
}

// AddRootComplex adds a new switch connecting CPU ports.
func (c *Connector) AddRootComplex(cpuPorts []sim.Port) (switchID int) {
	switchID = c.connector.AddSwitchWithName("RootComplex")

	c.PlugInDevice(switchID, cpuPorts)

	return switchID
}

// AddPCIeSwitch adds a new switch connecting from an existing switch.
func (c *Connector) AddPCIeSwitch() (switchID int) {
	switchID = c.connector.AddSwitchWithName(
		fmt.Sprintf("PCIeSwitch%d", len(c.pcieSwitches)))

	c.pcieSwitches[switchID] = true

	return switchID
}

// ConnectSwitchesWithPCIeLink connects two switches with a PCIe link.
func (c *Connector) ConnectSwitchesWithPCIeLink(switchAID, switchBID int) {
	c.connector.ConnectSwitches(switchAID, switchBID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.pcieSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.pcieSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       false,
				Frequency:     c.freq,
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

// PlugInDevice connects a series of ports to a switch.
func (c *Connector) PlugInDevice(
	pcieSwitchID int,
	devicePorts []sim.Port,
) (deviceID int) {
	deviceID = len(c.devices)

	deviceSwitchID := c.connector.AddSwitchWithName(
		fmt.Sprintf("DeviceSwitch%d", deviceID))
	nvlinkSwitchID := c.connector.AddSwitchWithName(
		fmt.Sprintf("NVLinkSwitch%d", deviceID))

	c.connectDeviceSwitchWithNVLinkSwitch(deviceSwitchID, nvlinkSwitchID)
	c.connectDeviceSwitchWithPCIeSwitch(pcieSwitchID, deviceSwitchID)
	c.connectDeviceWithDeviceSwitch(deviceSwitchID, devicePorts)

	deviceNode := &deviceNode{
		deviceSwitchID: deviceSwitchID,
		nvlinkSwitchID: nvlinkSwitchID,
	}
	c.devices = append(c.devices, deviceNode)

	return deviceID
}

func (c *Connector) connectDeviceSwitchWithNVLinkSwitch(
	deviceSwitchID int,
	nvlinkSwitchID int,
) {
	c.connector.ConnectSwitches(deviceSwitchID, nvlinkSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       true,
				Frequency:     c.freq,
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

func (c *Connector) connectDeviceSwitchWithPCIeSwitch(
	pcieSwitchID int,
	deviceSwitchID int,
) {
	c.connector.ConnectSwitches(deviceSwitchID, pcieSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          1,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       false,
				Frequency:     c.freq,
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

func (c *Connector) connectDeviceWithDeviceSwitch(
	deviceSwitchID int,
	devicePorts []sim.Port,
) {
	c.connector.ConnectDevice(deviceSwitchID, devicePorts,
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
				Latency:          c.pcieSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:   true,
				Frequency: c.freq,
			},
		})
}

// ConnectDevicesWithNVLink establishes an NVLink Connection.
func (c *Connector) ConnectDevicesWithNVLink(
	deviceA, deviceB int,
	numLink int,
) {
	deviceANode := c.devices[deviceA]
	deviceBNode := c.devices[deviceB]

	freq := math.Round(float64(c.nvlinkBandwidth) / float64(c.flitByteSize))

	c.connector.ConnectSwitches(
		deviceANode.nvlinkSwitchID,
		deviceBNode.nvlinkSwitchID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.nvlinkSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.nvlinkSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       false,
				Frequency:     sim.Freq(freq),
				NumStage:      20,
				CyclePerStage: 1,
				PipelineWidth: numLink,
			},
		})
}

// CreateEthernetSwitch creates a ethernet switch.
func (c *Connector) CreateEthernetSwitch() (switchID int) {
	switchID = c.connector.AddSwitch()

	c.ethernetSwitches[switchID] = true

	return switchID
}

// ConnectSwitchesWithEthernetLink establishes a ethernet link between two
// switches.
func (c *Connector) ConnectSwitchesWithEthernetLink(switchAID, switchBID int) {
	freq := math.Round(float64(c.ethernetBandwidth) / float64(c.flitByteSize))
	c.connector.ConnectSwitches(
		switchAID,
		switchBID,
		networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.ethernetSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			RightEndParam: networkconnector.LinkEndSwitchParameter{
				IncomingBufSize:  16,
				OutgoingBufSize:  16,
				Latency:          c.ethernetSwitchLatency,
				NumInputChannel:  1,
				NumOutputChannel: 1,
			},
			LinkParam: networkconnector.LinkParameter{
				IsIdeal:       false,
				Frequency:     sim.Freq(freq),
				NumStage:      10000,
				CyclePerStage: 1,
				PipelineWidth: 1,
			},
		})
}

// EstablishRoute populates the routing tables in the network.
func (c *Connector) EstablishRoute() {
	c.connector.EstablishRoute()
}
