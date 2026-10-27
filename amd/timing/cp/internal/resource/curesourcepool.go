package resource

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// DispatchableCU handles dispatch resource
type DispatchableCU interface {
	// DispatchingPort returns the port that the dispatcher can dispatch workgroups to.
	DispatchingPort() messaging.RemotePort

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
}

// CUResourcePoolImpl centralizes the resources of CUs.
type CUResourcePoolImpl struct {
	registeredCUs map[messaging.RemotePort]bool
	cus           []CUResource
}

// NewCUResourcePool returns a CUResourcePoll
func NewCUResourcePool() *CUResourcePoolImpl {
	p := &CUResourcePoolImpl{
		registeredCUs: make(map[messaging.RemotePort]bool),
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
	if _, found := p.registeredCUs[cu.DispatchingPort()]; found {
		return
	}

	state := &CUState{Reserved: make(map[uint64][]WfLocation)}
	r := &CUResourceImpl{state: func() *CUState { return state }}

	r.config.Port = cu.DispatchingPort()
	r.state().FreeWavefronts = cu.WfPoolSizes()
	p.createSRegMask(r, cu)
	p.createVRegMasks(r, cu)
	p.createLDSMask(r, cu)

	p.cus = append(p.cus, r)
	p.registeredCUs[cu.DispatchingPort()] = true
}

func (p *CUResourcePoolImpl) createSRegMask(
	r *CUResourceImpl,
	u DispatchableCU,
) {
	r.config.ScalarCount = u.SRegCount()

	if r.config.ScalarCount < 0 {
		r.state().Scalar = Mask{Unlimited: true}
		return
	}

	p.countMustBeAMultipleOfGranularity(r.config.ScalarCount, 16)
	r.state().Scalar = newResourceMask(r.config.ScalarCount / 16)
}

func (p *CUResourcePoolImpl) createVRegMasks(
	r *CUResourceImpl,
	u DispatchableCU,
) {
	r.config.VectorCounts = u.VRegCounts()

	for i := 0; i < len(r.config.VectorCounts); i++ {
		if r.config.VectorCounts[i] < 0 {
			r.state().Vector = append(r.state().Vector, Mask{Unlimited: true})
			continue
		}

		p.countMustBeAMultipleOfGranularity(
			r.config.VectorCounts[i], 4*64)
		r.state().Vector = append(r.state().Vector,
			newResourceMask(r.config.VectorCounts[i]/4/64))
	}
}

func (p *CUResourcePoolImpl) createLDSMask(
	r *CUResourceImpl,
	u DispatchableCU,
) {
	r.config.LDSCapacity = u.LDSBytes()

	if r.config.LDSCapacity < 0 {
		r.state().LDS = Mask{Unlimited: true}
		return
	}

	p.countMustBeAMultipleOfGranularity(r.config.LDSCapacity, 256)
	r.state().LDS = newResourceMask(r.config.LDSCapacity / 256)
}

func (p *CUResourcePoolImpl) countMustBeAMultipleOfGranularity(
	count, granularity int,
) {
	if count%granularity != 0 {
		panic("the count is not a multiple of the granularity")
	}
}
