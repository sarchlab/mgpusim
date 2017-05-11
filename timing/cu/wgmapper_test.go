package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

func assertAllResourcesFree(wgMapper *WGMapperImpl) {
	Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(10))
	Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(10))
	Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(10))
	Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(10))
	Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(256))
	Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(128))
	Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(64))
	Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(64))
	Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(64))
	Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(64))
}

var _ = Describe("WGMapper", func() {
	var (
		wgMapper *WGMapperImpl
		grid     *kernels.Grid
		status   *timing.KernelDispatchStatus
		co       *insts.HsaCo
	)

	BeforeEach(func() {
		wgMapper = NewWGMapper(4)
		grid = prepareGrid()
		status = timing.NewKernelDispatchStatus()
		status.Grid = grid
		co = insts.NewHsaCo()
		status.CodeObject = co
	})

	It("should send NACK if too many Wavefronts", func() {
		// Each SIMD is running 8 wf in each SIMD. 8 more wfs can handle.
		for i := 0; i < 4; i++ {
			wgMapper.WfPoolFreeCount[i] = 2
		}

		req := timing.NewMapWGReq(nil, nil, 0, grid.WorkGroups[0], co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(2))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(2))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(2))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(2))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(256))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(128))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(64))
	})

	It("should send NACK to the dispatcher if too many SReg", func() {
		// 128 groups in total, 125 groups occupied.
		// 3 groups are free -> 48 registers available
		wgMapper.SGprMask.SetStatus(0, 125, AllocStatusReserved)

		// 10 Wfs, 64 SGPRs per wf. That is 640 in total
		co.WFSgprCount = 64
		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(10))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(256))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(3))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(64))

	})

	It("should send NACK to the dispatcher if too large LDS", func() {
		// 240 units occupied, 16 units left -> 4096 Bytes available
		wgMapper.LDSMask.SetStatus(0, 240, AllocStatusReserved)

		co.WGGroupSegmentByteSize = 8192
		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(10))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(16))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(128))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(64))
	})

	It("should send NACK if too many VGPRs", func() {
		// 64 units occupied, 4 units available, 4 * 4 = 16 units
		wgMapper.VGprMask[0].SetStatus(0, 60, AllocStatusReserved)
		wgMapper.VGprMask[1].SetStatus(0, 60, AllocStatusReserved)
		wgMapper.VGprMask[2].SetStatus(0, 60, AllocStatusReserved)
		wgMapper.VGprMask[3].SetStatus(0, 60, AllocStatusReserved)

		co.WFSgprCount = 20
		co.WGGroupSegmentByteSize = 256
		co.WIVgprCount = 20

		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(10))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(256))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(128))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(4))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(4))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(4))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(4))
	})

	It("should send NACK if not all Wavefront can fit the VGPRs requirement", func() {
		// SIMD 0 and 1 do not have enouth VGPRs
		wgMapper.VGprMask[0].SetStatus(0, 60, AllocStatusReserved)
		wgMapper.VGprMask[1].SetStatus(0, 60, AllocStatusReserved)
		wgMapper.WfPoolFreeCount[2] = 2
		wgMapper.WfPoolFreeCount[3] = 2

		co.WIVgprCount = 102
		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(2))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(2))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(256))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(128))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(4))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(4))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(64))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(64))
	})

	It("should reserve resources and send ACK back if all requirement satisfy", func() {
		co.WIVgprCount = 20
		co.WFSgprCount = 16
		co.WGGroupSegmentByteSize = 1024

		wg := grid.WorkGroups[0]
		req := timing.NewMapWGReq(nil, nil, 10, wg, co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeTrue())
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(118))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusReserved)).To(Equal(10))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(252))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusReserved)).To(Equal(4))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(49))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusReserved)).To(Equal(15))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(49))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusReserved)).To(Equal(15))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(54))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusReserved)).To(Equal(10))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(54))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusReserved)).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(7))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(7))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(8))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(8))

		for i := 0; i < len(wg.Wavefronts); i++ {
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].SIMDID).To(
				Equal(i % 4))
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].SGPROffset).To(
				Equal(i * 64))
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].LDSOffset).To(
				Equal(0))
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].VGPROffset).To(
				Equal((i / 4) * 20 * 64 * 4))
		}
	})

	It("should reserve resources if resources are not aligned with granularity", func() {
		co.WIVgprCount = 18
		co.WFSgprCount = 14
		co.WGGroupSegmentByteSize = 900

		wg := grid.WorkGroups[0]
		req := timing.NewMapWGReq(nil, nil, 10, wg, co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeTrue())
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusFree)).To(Equal(118))
		Expect(wgMapper.SGprMask.StatusCount(AllocStatusReserved)).To(Equal(10))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusFree)).To(Equal(252))
		Expect(wgMapper.LDSMask.StatusCount(AllocStatusReserved)).To(Equal(4))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusFree)).To(Equal(49))
		Expect(wgMapper.VGprMask[0].StatusCount(AllocStatusReserved)).To(Equal(15))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusFree)).To(Equal(49))
		Expect(wgMapper.VGprMask[1].StatusCount(AllocStatusReserved)).To(Equal(15))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusFree)).To(Equal(54))
		Expect(wgMapper.VGprMask[2].StatusCount(AllocStatusReserved)).To(Equal(10))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusFree)).To(Equal(54))
		Expect(wgMapper.VGprMask[3].StatusCount(AllocStatusReserved)).To(Equal(10))
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(7))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(7))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(8))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(8))

		for i := 0; i < len(wg.Wavefronts); i++ {
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].SIMDID).To(Equal(i % 4))
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].SGPROffset).To(Equal(i * 64))
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].LDSOffset).To(Equal(0))
			Expect(req.WfDispatchMap[wg.Wavefronts[i]].VGPROffset).To(
				Equal((i / 4) * 20 * 64 * 4))
		}
	})

	It("should support non-standard CU size", func() {
		wgMapper.SetWfPoolSizes([]int{10, 10, 8, 8, 8})

		co.WIVgprCount = 20

		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], co)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeTrue())
		Expect(wgMapper.WfPoolFreeCount[0]).To(Equal(8))
		Expect(wgMapper.WfPoolFreeCount[1]).To(Equal(8))
		Expect(wgMapper.WfPoolFreeCount[2]).To(Equal(6))
		Expect(wgMapper.WfPoolFreeCount[3]).To(Equal(6))
		Expect(wgMapper.WfPoolFreeCount[4]).To(Equal(6))
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
		status.CodeObject = co
		req := timing.NewMapWGReq(nil, nil, 0, wg, co)

		managedWG := NewWorkGroup(wg, req)

		wgMapper.MapWG(req)
		for wf, info := range req.WfDispatchMap {
			managedWf := new(Wavefront)
			managedWf.Wavefront = wf
			managedWf.LDSOffset = info.LDSOffset
			managedWf.SIMDID = info.SIMDID
			managedWf.SRegOffset = info.SGPROffset
			managedWf.VRegOffset = info.VGPROffset
			managedWG.Wfs = append(managedWG.Wfs, managedWf)
		}

		wgMapper.UnmapWG(managedWG)

		assertAllResourcesFree(wgMapper)
	})
})
