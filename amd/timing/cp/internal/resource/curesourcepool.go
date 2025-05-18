package resource

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// DispatchableCU handles dispatch resource
type DispatchableCU interface {
	// DispatchingPort returns the port that the dispatcher can dispatch workgroups to.
	DispatchingPort() sim.RemotePort

	// WfPoolSizes returns an array of how many wavefront each wavefront pool
	// can hold. -1 is unlimited.
	WfPoolSizes() []int

	// VRegCounts returns how many vector registers are there in a vector
	// register file. It returns an array as the numbers represent the number of
	// vector registers in each SIMD unit. The size returned by this function
	// must equal to the size from the WfPoolSizes. -1 is unlimited.
	VRegCounts() []int

	// SRegCount returns the number of scalar registers. -1 means unlimited.
	SRegCount() int

	// LDSBytes returns the number of bytes in the LDS storage. -1 is unlimited.
	LDSBytes() int
}

// CUResourcePool centralized all the CU resources.
type CUResourcePool interface {
	NumCU() int
	GetCU(i int) CUResource
	RegisterCU(cu DispatchableCU)
}

// CUResourcePoolImpl centralizes the resources of CUs.
type CUResourcePoolImpl struct {
	registeredCUs map[DispatchableCU]bool
	cus           []CUResource
}

// NewCUResourcePool returns a CUResourcePoll
func NewCUResourcePool() *CUResourcePoolImpl {
	p := &CUResourcePoolImpl{
		registeredCUs: make(map[DispatchableCU]bool),
	}
	return p
}

// NumCU returns the total number of Compute Units.
func (p *CUResourcePoolImpl) NumCU() int {
	return len(p.cus)
}

// GetCU returns the i-th CU.
func (p *CUResourcePoolImpl) GetCU(i int) CUResource {
	return p.cus[i]
}

// RegisterCU puts the CU's resources into the resource pool.
func (p *CUResourcePoolImpl) RegisterCU(cu DispatchableCU) {
	if _, found := p.registeredCUs[cu]; found {
		return
	}

	r := &CUResourceImpl{
		reservedWGs: make(map[*kernels.WorkGroup][]WfLocation),
	}

	r.port = cu.DispatchingPort()
	r.wfPoolFreeCount = cu.WfPoolSizes()
	p.createSRegMask(r, cu)
	p.createVRegMasks(r, cu)
	p.createLDSMask(r, cu)

	p.cus = append(p.cus, r)
	p.registeredCUs[cu] = true
}

func (p *CUResourcePoolImpl) createSRegMask(
	r *CUResourceImpl,
	u DispatchableCU,
) {
	r.sregCount = u.SRegCount()
	r.sregGranularity = 16

	if r.sregCount < 0 {
		r.sregMask = &unlimitedResourceMask{}
		return
	}

	p.countMustBeAMultipleOfGranularity(r.sregCount, r.sregGranularity)
	r.sregMask = newResourceMask(r.sregCount / r.sregGranularity)
}

func (p *CUResourcePoolImpl) createVRegMasks(
	r *CUResourceImpl,
	u DispatchableCU,
) {
	r.vregCounts = u.VRegCounts()
	r.vregGranularity = 4

	for i := 0; i < len(r.vregCounts); i++ {
		if r.vregCounts[i] < 0 {
			r.vregMasks = append(r.vregMasks, &unlimitedResourceMask{})
			continue
		}

		p.countMustBeAMultipleOfGranularity(
			r.vregCounts[i], r.vregGranularity*64)
		r.vregMasks = append(r.vregMasks,
			newResourceMask(r.vregCounts[i]/r.vregGranularity/64))
	}
}

func (p *CUResourcePoolImpl) createLDSMask(
	r *CUResourceImpl,
	u DispatchableCU,
) {
	r.ldsByteSize = u.LDSBytes()
	r.ldsGranularity = 256

	if r.ldsByteSize < 0 {
		r.ldsMask = &unlimitedResourceMask{}
		return
	}

	p.countMustBeAMultipleOfGranularity(r.ldsByteSize, r.ldsGranularity)
	r.ldsMask = newResourceMask(r.ldsByteSize / r.ldsGranularity)
}

func (p *CUResourcePoolImpl) countMustBeAMultipleOfGranularity(
	count, granularity int,
) {
	if count%granularity != 0 {
		panic("the count is not a multiple of the granularity")
	}
}
