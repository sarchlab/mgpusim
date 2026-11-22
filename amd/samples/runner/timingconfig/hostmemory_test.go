package timingconfig

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

const (
	hostClientWindow = 256
	hostLineBytes    = 64
)

// hostTraffic is the address stream and request budget shared by clients.
type hostTraffic struct {
	next, stride uint64
	budget       int
	done         int
	first, last  timing.VTimeInPicoSec
}

// hostClient is a closed-loop reader of the host memory.
type hostClient struct {
	s        *simulation.Simulation
	name     string
	port     messaging.Port
	traffic  *hostTraffic
	sentAt   map[uint64]timing.VTimeInPicoSec
	rtts     []timing.VTimeInPicoSec
	received int
	hold     bool
	onRsp    func()
}

func newHostClient(s *simulation.Simulation, name string, capacity int, traffic *hostTraffic) *hostClient {
	c := &hostClient{s: s, name: name, traffic: traffic, sentAt: map[uint64]timing.VTimeInPicoSec{}}
	c.port = twowaybuffered.MakeBuilder().WithSimulation(s).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: capacity, OutgoingCapacity: hostClientWindow}).Build()
	c.port.BindOwner(c, name+".Port")
	s.RegisterPort(c.port)
	return c
}

func (c *hostClient) Name() string                  { return c.name }
func (*hostClient) BindPort(string, messaging.Port) {}
func (*hostClient) NotifyPortFree(messaging.Port)   {}

func (c *hostClient) issue() {
	if c.traffic.budget == 0 {
		return
	}
	c.traffic.budget--
	addr := c.traffic.next
	c.traffic.next += c.traffic.stride
	id := c.s.NewID()
	c.sentAt[id] = c.s.Engine().CurrentTime()
	c.port.Send(messaging.Msg{ID: id, Src: c.port.AsRemote(), Dst: cpuMemoryTop, TrafficBytes: 12,
		Payload: memprotocol.ReadReq{Address: addr, AccessByteSize: hostLineBytes}})
}

func (c *hostClient) NotifyRecv(port messaging.Port) {
	if c.hold {
		return
	}
	msg, _ := port.RetrieveIncoming()
	now := c.s.Engine().CurrentTime()
	c.received++
	if start, ok := c.sentAt[msg.RspTo]; ok {
		c.rtts = append(c.rtts, now-start)
		delete(c.sentAt, msg.RspTo)
		if c.traffic.done == 0 {
			c.traffic.first = now
		}
		c.traffic.done++
		c.traffic.last = now
	}
	if c.onRsp != nil {
		c.onRsp()
	}
	c.issue()
}

func (c *hostClient) meanRTTNs() float64 {
	var sum float64
	for _, rtt := range c.rtts {
		sum += float64(rtt) / 1000
	}
	return sum / float64(len(c.rtts))
}

// hostBench is a real platform network with its host memory and probe clients
// in place of the host components and GPUs.
type hostBench struct {
	s       *simulation.Simulation
	traffic *hostTraffic
	host    []*hostClient
	gpus    []*hostClient
}

func buildHostBench(t *testing.T, gpuType string, numGPUs, numHost, hostCapacity int) *hostBench {
	t.Helper()
	return buildHostBenchWithBinding(t, gpuType, numGPUs, numHost, hostCapacity, interDeviceNetwork.BindHostPort)
}

// buildHostBenchWithBinding selects how host clients attach to the Host Fabric.
func buildHostBenchWithBinding(t *testing.T, gpuType string, numGPUs, numHost, hostCapacity int,
	bindHost func(interDeviceNetwork, messaging.Port),
) *hostBench {
	t.Helper()
	s := simulation.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	b := MakeBuilder().WithSimulation(s).WithGPUType(gpuType).WithNumGPUs(numGPUs)
	b.globalStorage = mem.MakeStorageBuilder().WithSimulation(s).WithCapacity(4 * mem.GB).Build("GlobalStorage")
	network := b.createInterDeviceNetwork()
	b.buildCPUMemory(network)

	bench := &hostBench{s: s, traffic: &hostTraffic{stride: hostLineBytes}}
	for i := range numHost {
		c := newHostClient(s, fmt.Sprintf("Host%d", i), hostCapacity, bench.traffic)
		bindHost(network, c.port)
		bench.host = append(bench.host, c)
	}
	for i := 1; i <= numGPUs; i++ {
		c := newHostClient(s, fmt.Sprintf("GPU%d", i), hostClientWindow, bench.traffic)
		network.BindGPUPort(i, c.port)
		bench.gpus = append(bench.gpus, c)
	}
	network.Connect()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	return bench
}

func (h *hostBench) saturate(t *testing.T, clients []*hostClient, requests int) float64 {
	t.Helper()
	h.traffic.budget = requests
	for _, c := range clients {
		for range hostClientWindow {
			c.issue()
		}
	}
	if err := h.s.Run(); err != nil {
		t.Fatal(err)
	}
	if h.traffic.done != requests {
		t.Fatalf("%d requests completed, %d issued, want %d",
			h.traffic.done, requests-h.traffic.budget, requests)
	}
	return float64((h.traffic.done-1)*hostLineBytes) / (float64(h.traffic.last-h.traffic.first) / 1000)
}

func (h *hostBench) readOnce(t *testing.T, c *hostClient) float64 {
	t.Helper()
	h.traffic.budget = 1
	c.issue()
	if err := h.s.Run(); err != nil {
		t.Fatal(err)
	}
	if len(c.rtts) != 1 {
		t.Fatalf("%s got %d responses", c.name, len(c.rtts))
	}
	return float64(c.rtts[0]) / 1000
}

var hostPlatforms = []struct {
	name string
	spec HostMemorySpec
	peak float64 // GB/s
	gpus int
	pcie float64 // GB/s of 64 B read data over all downstream PCIe links
}{
	// A 64 B response is 3 flits of 34 B, over 4 PCIe switch links.
	{"r9nano", R9NanoHostMemorySpec(), 76.8, 8, 4 * 64.0 / 3},
	// A 64 B response is 2 flits of 68 B, over 8 PCIe switch links.
	{"mi300x", MI300XHostMemorySpec(), 460.8, 16, 8 * 64.0 / 2},
}

// Clients on fabric-wide links show the peak of the memory and its endpoint.
func TestHostMemoryPeakBandwidthFromHostFabric(t *testing.T) {
	for _, p := range hostPlatforms {
		t.Run(p.name, func(t *testing.T) {
			bench := buildHostBenchWithBinding(t, p.name, 1, 32, hostClientWindow,
				interDeviceNetwork.BindHostMemoryPort)
			got := bench.saturate(t, bench.host, 40000)
			t.Logf("saturated bandwidth %.1f GB/s, configured peak %.1f GB/s", got, p.peak)
			if got < 0.97*p.peak || got > 1.01*p.peak {
				t.Fatalf("bandwidth %.1f GB/s, want about %.1f GB/s", got, p.peak)
			}
		})
	}
}

// GPU DMA cannot exceed the DRAM peak or the PCIe link capacity. It stays below
// both because the flits of a response to a one-flit-wide link leave a wider
// switch input one per cycle and block the flits behind them.
func TestHostMemoryBandwidthThroughPCIe(t *testing.T) {
	for _, p := range hostPlatforms {
		t.Run(p.name, func(t *testing.T) {
			bench := buildHostBench(t, p.name, p.gpus, 0, hostClientWindow)
			got := bench.saturate(t, bench.gpus, 12000)
			limit := min(p.peak, p.pcie)
			t.Logf("GPU DMA %.1f GB/s; DRAM peak %.1f, PCIe capacity %.1f GB/s", got, p.peak, p.pcie)
			if got > 1.01*limit {
				t.Fatalf("bandwidth %.1f GB/s exceeds the limit %.1f GB/s", got, limit)
			}
			if got < 0.4*limit {
				t.Fatalf("bandwidth %.1f GB/s is far below the limit %.1f GB/s", got, limit)
			}
		})
	}
}

// A host client reaches the memory without the root complex. A GPU pays a PCIe
// switch and the root complex in each direction.
func TestHostMemoryPathLatency(t *testing.T) {
	const pcieRoundTripNs = 2 * (140 + 140)
	for _, p := range hostPlatforms {
		t.Run(p.name, func(t *testing.T) {
			host := buildHostBench(t, p.name, 2, 1, hostClientWindow)
			hostNs := host.readOnce(t, host.host[0])
			gpu := buildHostBench(t, p.name, 2, 1, hostClientWindow)
			gpuNs := gpu.readOnce(t, gpu.gpus[0])
			t.Logf("idle read: host %.1f ns, GPU %.1f ns", hostNs, gpuNs)
			if hostNs < hostMemoryLatencyNs || hostNs > hostMemoryLatencyNs+40 {
				t.Errorf("host read takes %.1f ns", hostNs)
			}
			if d := gpuNs - hostNs; d < pcieRoundTripNs || d > pcieRoundTripNs+40 {
				t.Errorf("GPU read is %.1f ns slower than a host read, want about %d ns", d, pcieRoundTripNs)
			}
		})
	}
}

// Page-table reads queue behind GPU DMA to the same channel.
func TestGPUDMADelaysPageTableReads(t *testing.T) {
	const (
		samples = 10
		// maxRequests outlasts the walker's reads at the rate of one channel
		// and, at the largest stride, stays below 100 MB of the backing store.
		// A walker that stops responding ends the run when it is used up.
		maxRequests = 100_000
	)
	for _, p := range hostPlatforms {
		t.Run(p.name, func(t *testing.T) {
			idle := buildHostBench(t, p.name, 2, 1, hostClientWindow)
			idleNs := idle.readOnce(t, idle.host[0])

			bench := buildHostBench(t, p.name, p.gpus, 1, hostClientWindow)
			bench.traffic.stride = uint64(p.spec.Channels) * hostLineBytes // every read hits one channel
			bench.traffic.budget = maxRequests
			walker := bench.host[0]
			walker.onRsp = func() {
				if len(walker.rtts) >= samples {
					bench.traffic.budget = 0
				}
			}
			for _, gpu := range bench.gpus {
				for range hostClientWindow {
					gpu.issue()
				}
			}
			walker.issue()
			if err := bench.s.Run(); err != nil {
				t.Fatal(err)
			}
			loadedNs := walker.meanRTTNs()
			t.Logf("page-table read: idle %.1f ns, under GPU DMA %.1f ns", idleNs, loadedNs)
			if len(walker.rtts) < samples || loadedNs < 2*idleNs {
				t.Fatalf("%d reads averaged %.1f ns, idle is %.1f ns", len(walker.rtts), loadedNs, idleNs)
			}
		})
	}
}

// A full IOMMU Top port stalls only its own endpoint, so the page-table
// responses that would let it drain are still delivered.
func TestFullHostPortDoesNotBlockMemoryResponses(t *testing.T) {
	for _, p := range hostPlatforms {
		t.Run(p.name, func(t *testing.T) {
			bench := buildHostBench(t, p.name, 1, 2, 1)
			top, memory := bench.host[0], bench.host[1]
			top.hold = true
			gpu := bench.gpus[0]
			for range 8 {
				gpu.port.Send(messaging.Msg{ID: bench.s.NewID(), Src: gpu.port.AsRemote(),
					Dst: top.port.AsRemote(), TrafficBytes: 12})
			}
			if err := bench.s.Run(); err != nil {
				t.Fatal(err)
			}
			if top.port.CanDeliver() {
				t.Fatal("the held port must be full")
			}
			if got := bench.readOnce(t, memory); got > hostMemoryLatencyNs+40 {
				t.Fatalf("read took %.1f ns behind a full port", got)
			}
		})
	}
}

// Every host port has its own endpoint, in both translation modes.
func TestEveryHostPortHasItsOwnEndpoint(t *testing.T) {
	for _, g := range gpuTypes {
		for _, fixed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fixed=%t", g.name, fixed), func(t *testing.T) {
				p := buildMigrationPlatform(t, g.name, func(b Builder) Builder {
					spec := DefaultPageTableSpec()
					spec.FixedLatency = fixed
					return b.WithPageTable(spec)
				})
				seen := map[messaging.Connection]string{}
				numHostPorts := 0
				for _, port := range p.sim.Ports() {
					name := port.Name()
					if !isHostNetworkPort(name) || port.Connection() == nil {
						continue
					}
					numHostPorts++
					if other, ok := seen[port.Connection()]; ok {
						t.Errorf("%s shares an endpoint with %s", name, other)
					}
					seen[port.Connection()] = name
				}
				if numHostPorts < 5 {
					t.Fatalf("found only %d host ports", numHostPorts)
				}
			})
		}
	}
}

func isHostNetworkPort(name string) bool {
	for _, prefix := range []string{"Driver.", "IOMMU.", "MMU.", "CPU.Memory."} {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
