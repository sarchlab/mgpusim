package resource

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

func assertAllResourcesFree(r *CUResourceImpl) {
	Expect(r.state().FreeWavefronts[0]).To(Equal(10))
	Expect(r.state().FreeWavefronts[1]).To(Equal(10))
	Expect(r.state().FreeWavefronts[2]).To(Equal(10))
	Expect(r.state().FreeWavefronts[3]).To(Equal(10))
	Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(256))
	Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(200))
	Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(64))
	Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(64))
	Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(64))
	Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(64))
}

var _ = Describe("cuResource", func() {
	var (
		r  *CUResourceImpl
		wg *kernels.WorkGroup
		co *insts.KernelCodeObject
	)

	BeforeEach(func() {
		state := &CUState{FreeWavefronts: []int{10, 10, 10, 10},
			Scalar:   newResourceMask(200),
			LDS:      newResourceMask(256),
			Vector:   []Mask{newResourceMask(64), newResourceMask(64), newResourceMask(64), newResourceMask(64)},
			Reserved: make(map[uint64][]WfLocation)}
		r = &CUResourceImpl{config: CUConfig{ScalarCount: 3200,
			VectorCounts: []int{256, 256, 256, 256},
			LDSCapacity:  65536},
			state: func() *CUState { return state }}

		wg = kernels.NewWorkGroup(testNewID())
		for i := 0; i < 10; i++ {
			wf := kernels.NewWavefront(testNewID())
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}

		co = &insts.KernelCodeObject{KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{}}
		wg.CodeObject = co
	})

	It("should send NACK if too many Wavefronts", func() {
		// Each SIMD is running 8 wf in each SIMD. 8 more wfs can handle.
		for i := 0; i < 4; i++ {
			r.state().FreeWavefronts[i] = 2
		}

		info, ok := r.ReserveResourceForWG(wg)

		Expect(info).To(BeEmpty())
		Expect(ok).To(BeFalse())
		Expect(r.state().FreeWavefronts[0]).To(Equal(2))
		Expect(r.state().FreeWavefronts[1]).To(Equal(2))
		Expect(r.state().FreeWavefronts[2]).To(Equal(2))
		Expect(r.state().FreeWavefronts[3]).To(Equal(2))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(64))
	})

	It("should send NACK to the dispatcher if too many SReg", func() {
		// 200 groups in total, 197 groups occupied.
		// 3 groups are free -> 48 registers available
		r.state().Scalar.setStatus(0, 197, allocStatusReserved)

		// 10 Wfs, 64 SGPRs per wf. That is 640 in total
		co.WFSgprCount = 64

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.state().FreeWavefronts[0]).To(Equal(10))
		Expect(r.state().FreeWavefronts[1]).To(Equal(10))
		Expect(r.state().FreeWavefronts[2]).To(Equal(10))
		Expect(r.state().FreeWavefronts[3]).To(Equal(10))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(3))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(64))

	})

	It("should send NACK to the dispatcher if too large LDS", func() {
		// 240 units occupied, 16 units left -> 4096 Bytes available
		r.state().LDS.setStatus(0, 240, allocStatusReserved)

		co.GroupSegmentByteSize = 8192

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.state().FreeWavefronts[0]).To(Equal(10))
		Expect(r.state().FreeWavefronts[1]).To(Equal(10))
		Expect(r.state().FreeWavefronts[2]).To(Equal(10))
		Expect(r.state().FreeWavefronts[3]).To(Equal(10))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(16))
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(64))
	})

	It("should send NACK if too many VGPRs", func() {
		// 64 units occupied, 4 units available, 4 * 4 = 16 units
		r.state().Vector[0].setStatus(0, 60, allocStatusReserved)
		r.state().Vector[1].setStatus(0, 60, allocStatusReserved)
		r.state().Vector[2].setStatus(0, 60, allocStatusReserved)
		r.state().Vector[3].setStatus(0, 60, allocStatusReserved)

		co.WFSgprCount = 20
		co.GroupSegmentByteSize = 256
		co.WIVgprCount = 20

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.state().FreeWavefronts[0]).To(Equal(10))
		Expect(r.state().FreeWavefronts[1]).To(Equal(10))
		Expect(r.state().FreeWavefronts[2]).To(Equal(10))
		Expect(r.state().FreeWavefronts[3]).To(Equal(10))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(4))
	})

	It("should send NACK if not all Wavefront can fit the VGPRs requirement", func() {
		// SIMD 0 and 1 do not have enouth VGPRs
		r.state().Vector[0].setStatus(0, 60, allocStatusReserved)
		r.state().Vector[1].setStatus(0, 60, allocStatusReserved)
		r.state().FreeWavefronts[2] = 2
		r.state().FreeWavefronts[3] = 2

		co.WIVgprCount = 102

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.state().FreeWavefronts[0]).To(Equal(10))
		Expect(r.state().FreeWavefronts[1]).To(Equal(10))
		Expect(r.state().FreeWavefronts[2]).To(Equal(2))
		Expect(r.state().FreeWavefronts[3]).To(Equal(2))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(64))
	})

	It("should reserve resources and send ACK back if all requirement satisfy", func() {
		co.WIVgprCount = 20
		co.WFSgprCount = 16
		co.GroupSegmentByteSize = 1024

		locations, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeTrue())
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(190))
		Expect(r.state().Scalar.statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(252))
		Expect(r.state().LDS.statusCount(allocStatusReserved)).To(Equal(4))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.state().Vector[0].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.state().Vector[1].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.state().Vector[2].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.state().Vector[3].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.state().FreeWavefronts[0]).To(Equal(7))
		Expect(r.state().FreeWavefronts[1]).To(Equal(7))
		Expect(r.state().FreeWavefronts[2]).To(Equal(8))
		Expect(r.state().FreeWavefronts[3]).To(Equal(8))

		for i := 0; i < len(wg.Wavefronts); i++ {
			Expect(locations[i].SIMDID).To(Equal(i % 4))
			Expect(locations[i].SGPROffset).To(Equal(i * 64))
			Expect(locations[i].LDSOffset).To(Equal(0))
			Expect(locations[i].VGPROffset).To(Equal((i / 4) * 20 * 4))
		}
	})

	It("should reserve resources if resources are not aligned with granularity", func() {
		co.WIVgprCount = 18
		co.WFSgprCount = 14
		co.GroupSegmentByteSize = 900

		locations, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeTrue())
		Expect(r.state().Scalar.statusCount(allocStatusFree)).To(Equal(190))
		Expect(r.state().Scalar.statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.state().LDS.statusCount(allocStatusFree)).To(Equal(252))
		Expect(r.state().LDS.statusCount(allocStatusReserved)).To(Equal(4))
		Expect(r.state().Vector[0].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.state().Vector[0].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.state().Vector[1].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.state().Vector[1].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.state().Vector[2].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.state().Vector[2].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.state().Vector[3].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.state().Vector[3].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.state().FreeWavefronts[0]).To(Equal(7))
		Expect(r.state().FreeWavefronts[1]).To(Equal(7))
		Expect(r.state().FreeWavefronts[2]).To(Equal(8))
		Expect(r.state().FreeWavefronts[3]).To(Equal(8))

		for i := 0; i < len(wg.Wavefronts); i++ {
			Expect(locations[i].SIMDID).To(Equal(i % 4))
			Expect(locations[i].SGPROffset).To(Equal(i * 64))
			Expect(locations[i].LDSOffset).To(Equal(0))
			Expect(locations[i].VGPROffset).To(Equal((i / 4) * 20 * 4))
		}
	})

	It("should support non-standard CU size", func() {
		r.state().FreeWavefronts = []int{10, 10, 8, 8, 8}
		r.config.VectorCounts = []int{256, 256, 256, 256, 256}
		r.state().Vector = []Mask{
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
		}

		co.WIVgprCount = 20

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeTrue())
		Expect(r.state().FreeWavefronts[0]).To(Equal(8))
		Expect(r.state().FreeWavefronts[1]).To(Equal(8))
		Expect(r.state().FreeWavefronts[2]).To(Equal(6))
		Expect(r.state().FreeWavefronts[3]).To(Equal(6))
		Expect(r.state().FreeWavefronts[4]).To(Equal(6))
	})

	It("should clear reservation when unmap wg", func() {
		wg := kernels.NewWorkGroup(testNewID())
		for i := 0; i < 10; i++ {
			wf := kernels.NewWavefront(testNewID())
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
		co.WIVgprCount = 16
		co.GroupSegmentByteSize = 1024
		co.WFSgprCount = 64
		wg.CodeObject = co

		r.ReserveResourceForWG(wg)
		r.FreeResourcesForWG(wg)
		assertAllResourcesFree(r)
	})
})
