package timingconfig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// deviceBytes is the memory of the CPU and of each GPU of the platform.
const deviceBytes = 4 << 30

// physicalLayout checks where the parts of a built platform find a physical
// address.
type physicalLayout struct {
	t           *testing.T
	p           migrationPlatform
	rt          *gpuruntime.Runtime
	numGPUs     int
	pageSize    uint64
	lowerMemory string
}

// The allocator of the runtime, the hardware of each GPU, and the routes
// between the GPUs must agree on the physical pages of a device: device i
// has [i*4 GiB, (i+1)*4 GiB). The first and the last page of every GPU are
// checked; a page migration can reach both, because the frames that pages
// leave are not used again.
func TestTheHardwareFindsTheFirstAndTheLastPageOfEveryGPU(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			p := buildMigrationPlatform(t, g.name, defaultPlatform)
			l := physicalLayout{
				t: t, p: p, rt: p.driver.Resources.Runtime, numGPUs: 2,
				pageSize: 1 << g.log2PageSize, lowerMemory: g.lowerMemory,
			}

			for gpu := 1; gpu <= l.numGPUs; gpu++ {
				l.mustHave(gpu)
			}
		})
	}
}

// owner returns the device whose allocator has a physical address.
func (l physicalLayout) owner(pAddr uint64) (device int, ok bool) {
	defer func() {
		if recover() != nil {
			device, ok = 0, false
		}
	}()

	return l.rt.DeviceIDByPAddr(pAddr), true
}

// mustHave checks the first and the last page of a GPU.
func (l physicalLayout) mustHave(gpu int) {
	l.t.Helper()

	first := uint64(gpu) * deviceBytes
	last := first + deviceBytes - l.pageSize

	// The allocator: the pages of the GPU, and no page around them.
	for _, pAddr := range []uint64{first, last, last + l.pageSize - 1} {
		if device, ok := l.owner(pAddr); !ok || device != gpu {
			l.t.Fatalf("the allocator has %#x on device %d (found: %v), want GPU %d", pAddr, device, ok, gpu)
		}
	}

	if device, ok := l.owner(first - 1); !ok || device != gpu-1 {
		l.t.Fatalf("the allocator has %#x, below GPU %d, on device %d (found: %v)", first-1, gpu, device, ok)
	}

	if device, ok := l.owner(last + l.pageSize); gpu == l.numGPUs && ok {
		l.t.Fatalf("the allocator has %#x, above the last GPU, on device %d", last+l.pageSize, device)
	}

	// The first page that the GPU hands out is the first of its range.
	ctx := l.rt.Init()
	l.rt.SelectGPU(ctx, gpu)
	ptr := l.rt.AllocateMemory(ctx, 1)

	page, found := l.p.driver.Resources.PageTable.Find(l.rt.QueuePID(l.rt.CreateCommandQueue(ctx)), uint64(ptr))
	if !found || page.PAddr != first {
		l.t.Fatalf("the first page of GPU %d is at %#x, want %#x", gpu, page.PAddr, first)
	}

	// The storage and the hardware, at both ends of the range.
	for _, pAddr := range []uint64{first, last + l.pageSize - 1} {
		l.mustStore(pAddr)
		l.mustRoute(gpu, pAddr)
	}
}

// mustStore checks that the global storage has an address.
func (l physicalLayout) mustStore(pAddr uint64) {
	l.t.Helper()

	defer func() {
		if r := recover(); r != nil {
			l.t.Fatalf("the global storage does not have %#x: %v", pAddr, r)
		}
	}()

	l.p.driver.Resources.GlobalStorage.Read(pAddr, 1)
}

// mustRoute checks that a GPU finds an address of its own in its L2 caches
// and in the memory below them, and that every other GPU sends accesses and
// page copies for the address to that GPU.
func (l physicalLayout) mustRoute(gpu int, pAddr uint64) {
	l.t.Helper()

	name := fmt.Sprintf("GPU[%d]", gpu)
	below := name + "." + l.lowerMemory + "["
	own := l.p.rdmas[name+".RDMA"].Spec

	if got := string(own.LocalModules.Find(pAddr)); !strings.HasPrefix(got, name+".L2Cache[") {
		l.t.Fatalf("%s finds its address %#x at %s, not in its L2 caches", name, pAddr, got)
	}

	if got := string(own.MigrationModules.Find(pAddr)); !strings.HasPrefix(got, below) {
		l.t.Fatalf("%s copies pages at %#x from and to %s", name, pAddr, got)
	}

	mover := l.p.movers[name+".DataMover"]
	if got := string(mover.Spec.Inside.Find(pAddr)); !strings.HasPrefix(got, below) {
		l.t.Fatalf("%s finds its address %#x at %s", mover.Name(), pAddr, got)
	}

	for peer := 1; peer <= l.numGPUs; peer++ {
		if peer != gpu {
			l.mustRouteFromPeer(fmt.Sprintf("GPU[%d]", peer), name, pAddr)
		}
	}
}

func (l physicalLayout) mustRouteFromPeer(peer, owner string, pAddr uint64) {
	l.t.Helper()

	engine := l.p.rdmas[peer+".RDMA"]

	if got := engine.Spec.LocalModules.Find(pAddr); got != engine.Ports.RDMARequestInside.AsRemote() {
		l.t.Fatalf("%s finds %#x of %s at %s, not at its RDMA engine", peer, pAddr, owner, got)
	}

	if got := engine.Spec.RemoteRDMAAddressTable.Find(pAddr); got != messaging.RemotePort(
		owner+".RDMA.RDMADataOutside") {
		l.t.Fatalf("%s sends accesses to %#x of %s to %s", peer, pAddr, owner, got)
	}

	mover := l.p.movers[peer+".DataMover"]
	if got := mover.Spec.Outside.Find(pAddr); got != messaging.RemotePort(
		owner+".RDMA.MigrationOutside") {
		l.t.Fatalf("%s copies pages at %#x of %s through %s", mover.Name(), pAddr, owner, got)
	}
}
