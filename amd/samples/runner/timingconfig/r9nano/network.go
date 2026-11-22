package r9nano

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/pcietree"
)

const (
	// pcieFlitSize is the number of bytes per flit of a PCIe Gen4 x16 link at
	// 1 GHz.
	pcieFlitSize = 34
	// gpusPerPCIeSwitch is the number of GPUs below each PCIe switch.
	gpusPerPCIeSwitch = 2
	// hostFabricWidth is the flits per cycle of the Host Fabric uplink and
	// memory link. A 64 B response is 3 flits after encoding, so 4 flits carry
	// about 85 GB/s, above the 76.8 GB/s of the default host memory.
	hostFabricWidth = 4
)

// Network connects the host and the GPUs with a PCIe tree. The host ports are
// on a Host Fabric below the root complex, and every two GPUs share a PCIe
// switch below the root complex.
type Network struct {
	tree          *pcietree.Tree
	switchLatency int
	rootLatency   int
	host          pcietree.HostPorts
	gpuPorts      [][]messaging.Port
}

// NewNetwork builds an empty PCIe tree for numGPUs GPUs with the given PCIe
// switch and root complex latencies in cycles.
func NewNetwork(s modeling.Simulation, numGPUs, switchLatency, rootLatency int) *Network {
	return &Network{
		tree:          pcietree.New(s, "PCIe", pcieFlitSize),
		switchLatency: switchLatency,
		rootLatency:   rootLatency,
		gpuPorts:      make([][]messaging.Port, numGPUs),
	}
}

// BindHostPort gives a host port its own endpoint on the Host Fabric.
func (n *Network) BindHostPort(port messaging.Port) { n.host.Bind(port) }

// BindHostMemoryPort gives the host memory port its own wide endpoint on the
// Host Fabric.
func (n *Network) BindHostMemoryPort(port messaging.Port) { n.host.BindMemory(port) }

// BindGPUPort plugs a port into the endpoint of the GPU with the given
// 1-based index.
func (n *Network) BindGPUPort(index int, port messaging.Port) {
	n.gpuPorts[index-1] = append(n.gpuPorts[index-1], port)
}

// Connect builds the tree from the ports bound so far.
func (n *Network) Connect() {
	root := n.tree.AddRootComplex(pcietree.RootConfig{Latency: n.rootLatency})
	numPCIeSwitches := (len(n.gpuPorts) + gpusPerPCIeSwitch - 1) / gpusPerPCIeSwitch
	n.host.Connect(n.tree, root, max(hostFabricWidth, numPCIeSwitches))

	var pcieSwitch pcietree.Group
	for i, ports := range n.gpuPorts {
		if i%gpusPerPCIeSwitch == 0 {
			pcieSwitch = n.tree.AddLocalGroup(root, pcietree.LocalGroupConfig{
				Name:        fmt.Sprintf("PCIeSwitch[%d]", i/gpusPerPCIeSwitch),
				Latency:     n.switchLatency,
				UplinkWidth: 1,
			})
		}
		pcieSwitch.ConnectDevice(fmt.Sprintf("GPU[%d].EndPoint", i+1), 1, ports)
	}
}
