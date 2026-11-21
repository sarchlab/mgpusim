package mi300x

import (
	"fmt"

	"github.com/sarchlab/akita/v5/noc/networkconnector"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

const (
	// networkFlitSize is the number of bytes per flit at 1 GHz of both a PCIe
	// Gen5 x16 link and an Infinity Fabric link. Both have a peak of 64 GB/s
	// per direction.
	networkFlitSize = 68
	// infinityFabricLatency is the cycles of one hop between two GPUs. It is
	// derived from the MI300A, where a GPU load from a neighboring APU takes
	// 344 ns more than a local load, or about 172 ns each way. It is not
	// calibrated on the MI300X.
	infinityFabricLatency = 160
	// gpusPerFabricDomain is the number of GPUs connected by Infinity Fabric.
	// GPUs in different domains communicate over PCIe.
	gpusPerFabricDomain = 4
	// gpusPerPCIeSwitch is the number of GPUs below each PCIe switch.
	gpusPerPCIeSwitch = 2
)

// Network connects the host and the GPUs. Every GPU has a local switch with
// one PCIe uplink and one Infinity Fabric link to each GPU of its domain.
// Every two GPUs share a PCIe switch below the root complex. Each device is one
// endpoint, so the ports are collected until Connect.
type Network struct {
	connector   *networkconnector.Connector
	pcieLatency int
	hostPorts   []messaging.Port
	gpuPorts    [][]messaging.Port
}

// NewNetwork builds an empty network for numGPUs GPUs. pcieLatency is the
// cycles to cross each PCIe switch or the root complex.
func NewNetwork(s modeling.Simulation, numGPUs, pcieLatency int) *Network {
	return &Network{
		connector: networkconnector.MakeBuilder().
			WithSimulation(s).
			WithFreq(timing.GHz).
			WithFlitSize(networkFlitSize).
			Build("InterDevice"),
		pcieLatency: pcieLatency,
		gpuPorts:    make([][]messaging.Port, numGPUs),
	}
}

// BindHostPort plugs a port into the endpoint of the host.
func (n *Network) BindHostPort(port messaging.Port) {
	n.hostPorts = append(n.hostPorts, port)
}

// BindGPUPort plugs a port into the endpoint of the GPU with the given
// 1-based index.
func (n *Network) BindGPUPort(index int, port messaging.Port) {
	n.gpuPorts[index-1] = append(n.gpuPorts[index-1], port)
}

// Connect builds the topology from the ports bound so far.
func (n *Network) Connect() {
	root := n.connector.AddSwitch(networkconnector.SwitchConfig{Name: "RootComplex"})
	n.connector.ConnectDevice(root, n.hostPorts, networkconnector.DeviceConfig{
		Name:           "Host",
		DeviceEndParam: deviceEnd(1),
		SwitchEndParam: switchEnd(1, n.pcieLatency),
	})

	gpuSwitches := n.connectGPUsToPCIe(root)
	n.connectFabricDomains(gpuSwitches)
}

// connectGPUsToPCIe attaches every GPU through its local switch. A flit pays
// the latency of the switch that it enters, so the local switch adds none:
// PCIe traffic pays only for the PCIe switches and the root complex, and
// Infinity Fabric traffic only for the fabric hop.
func (n *Network) connectGPUsToPCIe(root int) []int {
	gpuSwitches := make([]int, len(n.gpuPorts))

	var pcieSwitch int
	for i, ports := range n.gpuPorts {
		if i%gpusPerPCIeSwitch == 0 {
			pcieSwitch = n.addPCIeSwitch(root, i/gpusPerPCIeSwitch)
		}

		gpuSwitches[i] = n.connector.AddSwitch(networkconnector.SwitchConfig{
			Name: fmt.Sprintf("GPU[%d].Switch", i+1),
		})
		n.connector.ConnectSwitches(gpuSwitches[i], pcieSwitch, networkconnector.SwitchToSwitchLinkParameter{
			LeftEndParam:  switchEnd(1, 0),
			RightEndParam: switchEnd(1, n.pcieLatency),
		})

		width := n.endpointWidth(i)
		n.connector.ConnectDevice(gpuSwitches[i], ports, networkconnector.DeviceConfig{
			Name:           fmt.Sprintf("GPU[%d].EndPoint", i+1),
			DeviceEndParam: deviceEnd(width),
			SwitchEndParam: switchEnd(width, 0),
		})
	}

	return gpuSwitches
}

func (n *Network) addPCIeSwitch(root, index int) int {
	id := n.connector.AddSwitch(networkconnector.SwitchConfig{
		Name: fmt.Sprintf("PCIeSwitch[%d]", index),
	})
	n.connector.ConnectSwitches(id, root, networkconnector.SwitchToSwitchLinkParameter{
		LeftEndParam:  switchEnd(1, n.pcieLatency),
		RightEndParam: switchEnd(1, n.pcieLatency),
	})

	return id
}

// connectFabricDomains fully connects the GPUs of each domain.
func (n *Network) connectFabricDomains(gpuSwitches []int) {
	for a := range gpuSwitches {
		for b := a + 1; b < len(gpuSwitches) && b/gpusPerFabricDomain == a/gpusPerFabricDomain; b++ {
			n.connector.ConnectSwitches(gpuSwitches[a], gpuSwitches[b], networkconnector.SwitchToSwitchLinkParameter{
				LeftEndParam:  switchEnd(1, infinityFabricLatency),
				RightEndParam: switchEnd(1, infinityFabricLatency),
			})
		}
	}
}

// endpointWidth lets a GPU use its PCIe link and all of its Infinity Fabric
// links at the same time.
func (n *Network) endpointWidth(gpu int) int {
	domain := gpu / gpusPerFabricDomain
	domainSize := min(len(n.gpuPorts)-domain*gpusPerFabricDomain, gpusPerFabricDomain)

	return domainSize // domainSize-1 fabric links plus one PCIe link.
}

func switchEnd(width, latency int) networkconnector.LinkEndSwitchParameter {
	return networkconnector.LinkEndSwitchParameter{InputWidth: width, OutputWidth: width, Latency: latency}
}

func deviceEnd(width int) networkconnector.LinkEndDeviceParameter {
	return networkconnector.LinkEndDeviceParameter{InputWidth: width, OutputWidth: width}
}
