package timingconfig

import (
	"fmt"
	"strings"
	"testing"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300x"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
)

// networkOverheadNs bounds the cycles of endpoints and switch stages that a
// message pays on top of the modeled link and switch latencies.
const networkOverheadNs = 30

// networkProbe is a device with one port per peer that records when each port
// last received a message.
type networkProbe struct {
	s        *simulation.Simulation
	name     string
	ports    []messaging.Port
	arrivals map[string]timing.VTimeInPicoSec
}

func newNetworkProbe(s *simulation.Simulation, name string, numPorts int) *networkProbe {
	p := &networkProbe{s: s, name: name, arrivals: map[string]timing.VTimeInPicoSec{}}
	for i := range numPorts {
		port := twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 512, OutgoingCapacity: 512}).Build()
		port.BindOwner(p, fmt.Sprintf("%s.Port[%d]", name, i))
		s.RegisterPort(port)
		p.ports = append(p.ports, port)
	}
	return p
}

func (p *networkProbe) Name() string { return p.name }

func (*networkProbe) BindPort(string, messaging.Port) {}

func (p *networkProbe) NotifyRecv(port messaging.Port) {
	p.arrivals[port.Name()] = p.s.Engine().CurrentTime()
	port.RetrieveIncoming()
}

func (*networkProbe) NotifyPortFree(messaging.Port) {}

func (p *networkProbe) send(port int, dst *networkProbe, dstPort, bytes int) {
	p.ports[port].Send(messaging.Msg{
		ID: p.s.NewID(), Src: p.ports[port].AsRemote(), Dst: dst.ports[dstPort].AsRemote(), TrafficBytes: bytes,
	})
}

// networkPlatform has a host and GPUs, numbered from 1, on a network.
type networkPlatform struct {
	s    *simulation.Simulation
	host *networkProbe
	gpus []*networkProbe
}

func buildNetworkPlatform(t *testing.T, gpuType string, numGPUs, portsPerDevice int) networkPlatform {
	t.Helper()
	s := simulation.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})

	var network interDeviceNetwork = r9nano.NewNetwork(s, numGPUs, 140, 140)
	if gpuType == "mi300x" {
		network = mi300x.NewNetwork(s, numGPUs, 140, 140)
	}

	p := networkPlatform{s: s, host: newNetworkProbe(s, "Host", portsPerDevice)}
	for _, port := range p.host.ports {
		network.BindHostPort(port)
	}
	for i := 1; i <= numGPUs; i++ {
		gpu := newNetworkProbe(s, fmt.Sprintf("GPU%d", i), portsPerDevice)
		for _, port := range gpu.ports {
			network.BindGPUPort(i, port)
		}
		p.gpus = append(p.gpus, gpu)
	}
	network.Connect()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	return p
}

func (p networkPlatform) device(index int) *networkProbe {
	if index == 0 {
		return p.host
	}
	return p.gpus[index-1]
}

func TestInterDeviceNetworkPathLatency(t *testing.T) {
	const (
		pcie   = 140
		fabric = 160
	)
	type path struct {
		name     string
		src, dst int // 0 is the host.
		wantNs   uint64
	}
	gpuHost := []path{{"gpu1-host", 1, 0, 2 * pcie}, {"host-gpu1", 0, 1, 2 * pcie}}
	r9PeerPaths := map[int][]path{
		2: {{"same-switch", 1, 2, pcie}},
		4: {{"same-switch", 3, 4, pcie}, {"across-root", 1, 3, 3 * pcie}},
		8: {{"same-switch", 7, 8, pcie}, {"across-root", 1, 8, 3 * pcie}},
	}
	miPeerPaths := map[int][]path{
		2: {{"fabric-same-pcie-switch", 1, 2, fabric}},
		4: {{"fabric-same-pcie-switch", 3, 4, fabric}, {"fabric-other-pcie-switch", 1, 4, fabric}},
		8: {
			{"fabric-domain-1", 1, 3, fabric}, {"fabric-domain-2", 5, 8, fabric},
			{"pcie-across-domains", 4, 5, 3 * pcie}, {"pcie-across-domains-reverse", 8, 1, 3 * pcie},
		},
	}
	for gpuType, peers := range map[string]map[int][]path{"r9nano": r9PeerPaths, "mi300x": miPeerPaths} {
		for _, numGPUs := range []int{1, 2, 4, 8} {
			for _, c := range append(append([]path{}, gpuHost...), peers[numGPUs]...) {
				t.Run(fmt.Sprintf("%s/%d/%s", gpuType, numGPUs, c.name), func(t *testing.T) {
					p := buildNetworkPlatform(t, gpuType, numGPUs, 1)
					src, dst := p.device(c.src), p.device(c.dst)
					src.send(0, dst, 0, 12)
					if err := p.s.Run(); err != nil {
						t.Fatal(err)
					}
					gotNs := uint64(dst.arrivals[dst.ports[0].Name()]) / 1000
					t.Logf("one-way latency: %d ns", gotNs)
					if gotNs < c.wantNs || gotNs > c.wantNs+networkOverheadNs {
						t.Fatalf("latency %d ns, want %d ns plus at most %d ns of overhead",
							gotNs, c.wantNs, networkOverheadNs)
					}
				})
			}
		}
	}
}

// A MI300X GPU can send one flit per cycle on each Infinity Fabric link, so
// interleaved single-flit messages to three peers take about as long as the
// same number of messages to one peer. The endpoint serializes the flits of a
// message, so this does not hold for large messages.
func TestMI300XEndpointDrivesAllFabricLinks(t *testing.T) {
	const (
		singleFlitBytes = 54
		messagesPerPeer = 256
	)

	finish := func(numPeers int) timing.VTimeInPicoSec {
		p := buildNetworkPlatform(t, "mi300x", 4, 3)
		for i := range numPeers {
			for range messagesPerPeer {
				p.gpus[0].send(i, p.gpus[i+1], i, singleFlitBytes)
			}
		}
		if err := p.s.Run(); err != nil {
			t.Fatal(err)
		}
		var last timing.VTimeInPicoSec
		for i := range numPeers {
			gpu := p.gpus[i+1]
			last = max(last, gpu.arrivals[gpu.ports[i].Name()])
		}
		return last
	}

	one, three := finish(1), finish(3)
	if float64(three) > 1.1*float64(one) {
		t.Fatalf("three peers take %d ps, one peer takes %d ps", three, one)
	}
}

// The IOMMU reads the page table from the one host memory that GPU DMA uses.
func TestIOMMUUsesTheSharedHostMemory(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			p := buildMigrationPlatform(t, g.name, defaultPlatform)
			if got := p.mmu.Spec.Memory.Find(p.mmu.Spec.PoolBase); got != cpuMemoryTop {
				t.Errorf("page walks go to %s", got)
			}
			var memories int
			for _, c := range p.sim.Components() {
				if strings.HasPrefix(c.Name(), "CPU.") {
					memories++
				}
			}
			if memories != 1 {
				t.Errorf("found %d CPU memory components, want 1", memories)
			}
		})
	}
}
