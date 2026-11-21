package r9nano

import (
	"github.com/sarchlab/akita/v5/noc/pcie"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

const (
	// pcieFlitSize is the number of bytes per flit of a PCIe Gen4 x16 link at
	// 1 GHz.
	pcieFlitSize = 34
	// gpusPerPCIeSwitch is the number of GPUs below each PCIe switch.
	gpusPerPCIeSwitch = 2
)

// Network connects the host and the GPUs with a PCIe tree: the host is on the
// root complex, and every two GPUs share a switch below it. Each device is one
// PCIe endpoint, so the ports are collected until Connect.
type Network struct {
	connector *pcie.Connector
	hostPorts *[]messaging.Port
	gpuPorts  [][]messaging.Port
}

// NewNetwork builds an empty PCIe tree for numGPUs GPUs with the given switch
// latency in cycles.
func NewNetwork(s modeling.Simulation, numGPUs, switchLatency int) Network {
	return Network{
		connector: pcie.MakeBuilder().
			WithSimulation(s).
			WithFreq(timing.GHz).
			WithFlitSize(pcieFlitSize).
			WithSwitchLatency(switchLatency).
			Build("PCIe"),
		hostPorts: new([]messaging.Port),
		gpuPorts:  make([][]messaging.Port, numGPUs),
	}
}

// BindHostPort plugs a port into the endpoint of the host.
func (n Network) BindHostPort(port messaging.Port) {
	*n.hostPorts = append(*n.hostPorts, port)
}

// BindGPUPort plugs a port into the endpoint of the GPU with the given
// 1-based index.
func (n Network) BindGPUPort(index int, port messaging.Port) {
	n.gpuPorts[index-1] = append(n.gpuPorts[index-1], port)
}

// Connect builds the tree from the ports bound so far.
func (n Network) Connect() {
	root := n.connector.AddRootComplex(*n.hostPorts)

	var pcieSwitch int
	for i, ports := range n.gpuPorts {
		if i%gpusPerPCIeSwitch == 0 {
			pcieSwitch = n.connector.AddSwitch(root)
		}
		n.connector.ConnectDevice(pcieSwitch, ports)
	}
}
