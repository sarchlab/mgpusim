package mi300x

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/pcietree"
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
	// hostFabricWidth is the flits per cycle of the Host Fabric uplink and
	// memory link. A 64 B response is 2 flits after encoding, so 16 flits carry
	// 512 GB/s, above the 460.8 GB/s of the default host memory.
	hostFabricWidth = 16
)

// Network connects the host and the GPUs. The host ports are on a Host Fabric
// below the root complex. Every GPU has a local switch with one PCIe uplink and
// one Infinity Fabric link to each GPU of its domain, and every two GPUs share
// a PCIe switch below the root complex.
type Network struct {
	tree        *pcietree.Tree
	pcieLatency int
	rootLatency int
	host        pcietree.HostPorts
	gpuPorts    [][]messaging.Port
}

// NewNetwork builds an empty network for numGPUs GPUs with the given PCIe
// switch and root complex latencies in cycles.
func NewNetwork(s modeling.Simulation, numGPUs, pcieLatency, rootLatency int) *Network {
	return &Network{
		tree:        pcietree.New(s, "InterDevice", networkFlitSize),
		pcieLatency: pcieLatency,
		rootLatency: rootLatency,
		gpuPorts:    make([][]messaging.Port, numGPUs),
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

// Connect builds the topology from the ports bound so far.
func (n *Network) Connect() {
	root := n.tree.AddRootComplex(pcietree.RootConfig{Latency: n.rootLatency})
	numPCIeSwitches := (len(n.gpuPorts) + gpusPerPCIeSwitch - 1) / gpusPerPCIeSwitch
	n.host.Connect(n.tree, root, max(hostFabricWidth, numPCIeSwitches))

	n.connectFabricDomains(n.connectGPUsToPCIe(root))
}

// connectGPUsToPCIe attaches every GPU through its local switch. A flit pays
// the latency of the switch that it enters, so the local switch adds none:
// PCIe traffic pays only for the PCIe switches and the root complex, and
// Infinity Fabric traffic only for the fabric hop.
func (n *Network) connectGPUsToPCIe(root pcietree.Group) []pcietree.Group {
	gpuSwitches := make([]pcietree.Group, len(n.gpuPorts))

	var pcieSwitch pcietree.Group
	for i, ports := range n.gpuPorts {
		if i%gpusPerPCIeSwitch == 0 {
			pcieSwitch = n.tree.AddLocalGroup(root, pcietree.LocalGroupConfig{
				Name:        fmt.Sprintf("PCIeSwitch[%d]", i/gpusPerPCIeSwitch),
				Latency:     n.pcieLatency,
				UplinkWidth: 1,
			})
		}

		gpuSwitches[i] = n.tree.AddLocalGroup(pcieSwitch, pcietree.LocalGroupConfig{
			Name:        fmt.Sprintf("GPU[%d].Switch", i+1),
			UplinkWidth: 1,
		})
		gpuSwitches[i].ConnectDevice(fmt.Sprintf("GPU[%d].EndPoint", i+1), n.endpointWidth(i), ports)
	}

	return gpuSwitches
}

// connectFabricDomains fully connects the GPUs of each domain.
func (n *Network) connectFabricDomains(gpuSwitches []pcietree.Group) {
	for a := range gpuSwitches {
		for b := a + 1; b < len(gpuSwitches) && b/gpusPerFabricDomain == a/gpusPerFabricDomain; b++ {
			pcietree.ConnectPeers(gpuSwitches[a], gpuSwitches[b], infinityFabricLatency)
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
