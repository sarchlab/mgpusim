package resource

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

// SetName names the shared pool before registering it as a simulation resource.
func (p *CUResourcePoolImpl) SetName(name string) { p.name = name }

// Name identifies the shared allocation state in a checkpoint.
func (p *CUResourcePoolImpl) Name() string { return p.name }

type cuConfig struct {
	Port                           messaging.RemotePort
	WavefrontPools                 int
	ScalarCount, ScalarGranularity int
	VectorCounts                   []int
	VectorGranularity              int
	LDSBytes, LDSGranularity       int
}

type maskState struct {
	Unlimited bool
	Next      int
	Mask      []allocStatus
}

type cuState struct {
	FreeWavefronts []int
	Scalar, LDS    maskState
	Vector         []maskState
	NextSIMD       int
	Reserved       map[uint64][]WfLocation
}

func snapshotMask(m resourceMask) maskState {
	switch m := m.(type) {
	case *resourceMaskImpl:
		return maskState{Mask: m.mask}
	case *unlimitedResourceMask:
		return maskState{Unlimited: true, Next: m.next}
	default:
		panic("resource: unknown allocation mask")
	}
}

func restoreMask(s maskState) resourceMask {
	if s.Unlimited {
		return &unlimitedResourceMask{next: s.Next}
	}
	return &resourceMaskImpl{mask: s.Mask}
}

func (p *CUResourcePoolImpl) config() []cuConfig {
	cfg := make([]cuConfig, len(p.cus))
	for i, cu := range p.cus {
		r := cu.(*CUResourceImpl)
		cfg[i] = cuConfig{
			r.port,
			len(r.wfPoolFreeCount),
			r.sregCount,
			r.sregGranularity,
			r.vregCounts,
			r.vregGranularity,
			r.ldsByteSize,
			r.ldsGranularity,
		}
	}
	return cfg
}

// SaveCheckpoint saves reservations by work-group ID, including unlimited
// allocation cursors. No original work-group pointers are required on load.
func (p *CUResourcePoolImpl) SaveCheckpoint(w io.Writer) error {
	state := make([]cuState, len(p.cus))
	for i, cu := range p.cus {
		r := cu.(*CUResourceImpl)
		s := cuState{
			FreeWavefronts: r.wfPoolFreeCount,
			Scalar:         snapshotMask(r.sregMask),
			LDS:            snapshotMask(r.ldsMask),
			NextSIMD:       r.nextSIMD,
			Reserved:       r.reservedWGs,
		}
		for _, m := range r.vregMasks {
			s.Vector = append(s.Vector, snapshotMask(m))
		}
		state[i] = s
	}
	return modeling.WriteCheckpoint(w, p.config(), state, nil)
}

// LoadCheckpoint restores the pool after checking its configured CU ordering.
func (p *CUResourcePoolImpl) LoadCheckpoint(r io.Reader) error {
	var state []cuState
	if err := modeling.ReadCheckpoint(r, p.config(), &state, nil); err != nil {
		return err
	}
	if len(state) != len(p.cus) {
		return fmt.Errorf("resource: CU count mismatch")
	}
	for i, s := range state {
		cu := p.cus[i].(*CUResourceImpl)
		if len(s.FreeWavefronts) != len(cu.wfPoolFreeCount) || len(s.Vector) != len(cu.vregMasks) {
			return fmt.Errorf("resource: SIMD count mismatch for CU %d", i)
		}
	}
	for i, s := range state {
		cu := p.cus[i].(*CUResourceImpl)
		cu.wfPoolFreeCount = s.FreeWavefronts
		cu.sregMask = restoreMask(s.Scalar)
		cu.ldsMask = restoreMask(s.LDS)
		for j, m := range s.Vector {
			cu.vregMasks[j] = restoreMask(m)
		}
		cu.nextSIMD = s.NextSIMD
		cu.reservedWGs = s.Reserved
	}
	return nil
}
