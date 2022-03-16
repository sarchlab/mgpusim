package resource

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

func assertAllResourcesFree(r *CUResourceImpl) {
	Expect(r.wfPoolFreeCount[0]).To(Equal(10))
	Expect(r.wfPoolFreeCount[1]).To(Equal(10))
	Expect(r.wfPoolFreeCount[2]).To(Equal(10))
	Expect(r.wfPoolFreeCount[3]).To(Equal(10))
	Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(256))
	Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(200))
	Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(64))
	Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(64))
	Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(64))
	Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(64))
}

var _ = Describe("cuResource", func() {
	var (
		r  *CUResourceImpl
		wg *kernels.WorkGroup
		co *insts.HsaCo
	)

	BeforeEach(func() {
		r = &CUResourceImpl{
			wfPoolFreeCount: []int{10, 10, 10, 10},
			sregCount:       3200,
			sregGranularity: 16,
			sregMask:        newResourceMask(3200 / 16),
			vregCounts:      []int{256, 256, 256, 256},
			vregGranularity: 4,
			vregMasks: []resourceMask{
				newResourceMask(256 / 4),
				newResourceMask(256 / 4),
				newResourceMask(256 / 4),
				newResourceMask(256 / 4),
			},
			ldsByteSize:    64 * 1024,
			ldsGranularity: 256,
			ldsMask:        newResourceMask(64 * 1024 / 256),
			reservedWGs:    make(map[*kernels.WorkGroup][]WfLocation),
		}

		wg = kernels.NewWorkGroup()
		for i := 0; i < 10; i++ {
			wf := kernels.NewWavefront()
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}

		co = insts.NewHsaCo()
		wg.CodeObject = co
	})

	It("should send NACK if too many Wavefronts", func() {
		// Each SIMD is running 8 wf in each SIMD. 8 more wfs can handle.
		for i := 0; i < 4; i++ {
			r.wfPoolFreeCount[i] = 2
		}

		info, ok := r.ReserveResourceForWG(wg)

		Expect(info).To(BeEmpty())
		Expect(ok).To(BeFalse())
		Expect(r.wfPoolFreeCount[0]).To(Equal(2))
		Expect(r.wfPoolFreeCount[1]).To(Equal(2))
		Expect(r.wfPoolFreeCount[2]).To(Equal(2))
		Expect(r.wfPoolFreeCount[3]).To(Equal(2))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(64))
	})

	It("should send NACK to the dispatcher if too many SReg", func() {
		// 200 groups in total, 197 groups occupied.
		// 3 groups are free -> 48 registers available
		r.sregMask.setStatus(0, 197, allocStatusReserved)

		// 10 Wfs, 64 SGPRs per wf. That is 640 in total
		co.WFSgprCount = 64

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.wfPoolFreeCount[0]).To(Equal(10))
		Expect(r.wfPoolFreeCount[1]).To(Equal(10))
		Expect(r.wfPoolFreeCount[2]).To(Equal(10))
		Expect(r.wfPoolFreeCount[3]).To(Equal(10))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(3))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(64))

	})

	It("should send NACK to the dispatcher if too large LDS", func() {
		// 240 units occupied, 16 units left -> 4096 Bytes available
		r.ldsMask.setStatus(0, 240, allocStatusReserved)

		co.WGGroupSegmentByteSize = 8192

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.wfPoolFreeCount[0]).To(Equal(10))
		Expect(r.wfPoolFreeCount[1]).To(Equal(10))
		Expect(r.wfPoolFreeCount[2]).To(Equal(10))
		Expect(r.wfPoolFreeCount[3]).To(Equal(10))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(16))
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(64))
	})

	It("should send NACK if too many VGPRs", func() {
		// 64 units occupied, 4 units available, 4 * 4 = 16 units
		r.vregMasks[0].setStatus(0, 60, allocStatusReserved)
		r.vregMasks[1].setStatus(0, 60, allocStatusReserved)
		r.vregMasks[2].setStatus(0, 60, allocStatusReserved)
		r.vregMasks[3].setStatus(0, 60, allocStatusReserved)

		co.WFSgprCount = 20
		co.WGGroupSegmentByteSize = 256
		co.WIVgprCount = 20

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.wfPoolFreeCount[0]).To(Equal(10))
		Expect(r.wfPoolFreeCount[1]).To(Equal(10))
		Expect(r.wfPoolFreeCount[2]).To(Equal(10))
		Expect(r.wfPoolFreeCount[3]).To(Equal(10))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(4))
	})

	It("should send NACK if not all Wavefront can fit the VGPRs requirement", func() {
		// SIMD 0 and 1 do not have enouth VGPRs
		r.vregMasks[0].setStatus(0, 60, allocStatusReserved)
		r.vregMasks[1].setStatus(0, 60, allocStatusReserved)
		r.wfPoolFreeCount[2] = 2
		r.wfPoolFreeCount[3] = 2

		co.WIVgprCount = 102

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeFalse())
		Expect(r.wfPoolFreeCount[0]).To(Equal(10))
		Expect(r.wfPoolFreeCount[1]).To(Equal(10))
		Expect(r.wfPoolFreeCount[2]).To(Equal(2))
		Expect(r.wfPoolFreeCount[3]).To(Equal(2))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(256))
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(200))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(4))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(64))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(64))
	})

	It("should reserve resources and send ACK back if all requirement satisfy", func() {
		co.WIVgprCount = 20
		co.WFSgprCount = 16
		co.WGGroupSegmentByteSize = 1024

		locations, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeTrue())
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(190))
		Expect(r.sregMask.statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(252))
		Expect(r.ldsMask.statusCount(allocStatusReserved)).To(Equal(4))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.vregMasks[0].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.vregMasks[1].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.vregMasks[2].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.vregMasks[3].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.wfPoolFreeCount[0]).To(Equal(7))
		Expect(r.wfPoolFreeCount[1]).To(Equal(7))
		Expect(r.wfPoolFreeCount[2]).To(Equal(8))
		Expect(r.wfPoolFreeCount[3]).To(Equal(8))

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
		co.WGGroupSegmentByteSize = 900

		locations, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeTrue())
		Expect(r.sregMask.statusCount(allocStatusFree)).To(Equal(190))
		Expect(r.sregMask.statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.ldsMask.statusCount(allocStatusFree)).To(Equal(252))
		Expect(r.ldsMask.statusCount(allocStatusReserved)).To(Equal(4))
		Expect(r.vregMasks[0].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.vregMasks[0].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.vregMasks[1].statusCount(allocStatusFree)).To(Equal(49))
		Expect(r.vregMasks[1].statusCount(allocStatusReserved)).To(Equal(15))
		Expect(r.vregMasks[2].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.vregMasks[2].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.vregMasks[3].statusCount(allocStatusFree)).To(Equal(54))
		Expect(r.vregMasks[3].statusCount(allocStatusReserved)).To(Equal(10))
		Expect(r.wfPoolFreeCount[0]).To(Equal(7))
		Expect(r.wfPoolFreeCount[1]).To(Equal(7))
		Expect(r.wfPoolFreeCount[2]).To(Equal(8))
		Expect(r.wfPoolFreeCount[3]).To(Equal(8))

		for i := 0; i < len(wg.Wavefronts); i++ {
			Expect(locations[i].SIMDID).To(Equal(i % 4))
			Expect(locations[i].SGPROffset).To(Equal(i * 64))
			Expect(locations[i].LDSOffset).To(Equal(0))
			Expect(locations[i].VGPROffset).To(Equal((i / 4) * 20 * 4))
		}
	})

	It("should support non-standard CU size", func() {
		r.wfPoolFreeCount = []int{10, 10, 8, 8, 8}
		r.vregCounts = []int{256, 256, 256, 256, 256}
		r.vregMasks = []resourceMask{
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
			newResourceMask(256 / 4),
		}

		co.WIVgprCount = 20

		_, ok := r.ReserveResourceForWG(wg)

		Expect(ok).To(BeTrue())
		Expect(r.wfPoolFreeCount[0]).To(Equal(8))
		Expect(r.wfPoolFreeCount[1]).To(Equal(8))
		Expect(r.wfPoolFreeCount[2]).To(Equal(6))
		Expect(r.wfPoolFreeCount[3]).To(Equal(6))
		Expect(r.wfPoolFreeCount[4]).To(Equal(6))
	})

	It("should clear reservation when unmap wg", func() {
		wg := kernels.NewWorkGroup()
		for i := 0; i < 10; i++ {
			wf := kernels.NewWavefront()
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
		co.WIVgprCount = 16
		co.WGGroupSegmentByteSize = 1024
		co.WFSgprCount = 64
		wg.CodeObject = co

		r.ReserveResourceForWG(wg)
		r.FreeResourcesForWG(wg)
		assertAllResourcesFree(r)
	})
})
