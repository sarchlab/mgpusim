package runtime

import (
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

//go:generate mockgen -destination "mock_internal_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v5/amd/runtime/internal MemoryAllocator
func TestRuntime(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "AMD Runtime")
}

type runtimeFixture struct {
	r   *Runtime
	ctx Context
	q   CommandQueue
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	t.Helper()
	s := sim.MakeBuilder().Build()
	storage := mem.MakeStorageBuilder().WithSimulation(s).WithCapacity(12 * mem.GB).Build("GlobalStorage")
	spec := DefaultRuntimeSpec()
	spec.Devices = []DeviceProperties{{CUCount: 4, DRAMSize: 4 * mem.GB}, {CUCount: 4, DRAMSize: 4 * mem.GB}}
	r := NewRuntime(s, "Runtime", spec, vm.NewPageTable(12), storage)
	ctx := r.Init()
	return &runtimeFixture{r: r, ctx: ctx, q: r.CreateCommandQueue(ctx)}
}
