package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/gcn3/timing/cu"
)

var _ = Describe("WGMapper", func() {
	var (
		wgMapper *cu.WGMapperImpl
		grid     *kernels.Grid
		status   *timing.KernelDispatchStatus
		co       *insts.HsaCo
	)

	BeforeEach(func() {
		wgMapper = cu.NewWGMapper(4)
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

		req := timing.NewMapWGReq(nil, nil, 0, grid.WorkGroups[0], status)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
	})

	It("should send NACK to the dispatcher if too many SReg", func() {
		// 128 groups in total, 125 groups occupied.
		// 3 groups are free -> 48 registers available
		wgMapper.SGprMask.SetStatus(0, 125, cu.AllocStatusReserved)

		// 10 Wfs, 64 SGPRs per wf. That is 640 in total
		co.WFSgprCount = 64
		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], status)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
	})

	It("should send NACK to the dispatcher if too large LDS", func() {
		// 240 units occupied, 16 units left -> 4096 Bytes available
		wgMapper.LDSMask.SetStatus(0, 240, cu.AllocStatusReserved)

		co.WGGroupSegmentByteSize = 8192
		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], status)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
	})

	It("should send NACK if too many VGPRs", func() {
		// 64 units occupied, 4 units available, 4 * 4 = 16 units
		wgMapper.VGprMask[0].SetStatus(0, 60, cu.AllocStatusReserved)
		wgMapper.VGprMask[1].SetStatus(0, 60, cu.AllocStatusReserved)
		wgMapper.VGprMask[2].SetStatus(0, 60, cu.AllocStatusReserved)
		wgMapper.VGprMask[3].SetStatus(0, 60, cu.AllocStatusReserved)

		co.WIVgprCount = 20

		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], status)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
	})

	It("should send NACK if not all Wavefront can fit the VGPRs requirement", func() {
		// SIMD 0 and 1 do not have enouth VGPRs
		wgMapper.VGprMask[0].SetStatus(0, 60, cu.AllocStatusReserved)
		wgMapper.VGprMask[1].SetStatus(0, 60, cu.AllocStatusReserved)
		wgMapper.WfPoolFreeCount[2] = 2
		wgMapper.WfPoolFreeCount[3] = 2

		co.WIVgprCount = 102
		req := timing.NewMapWGReq(nil, nil, 10, grid.WorkGroups[0], status)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeFalse())
	})

	It("should reserve resources and send ACK back if all requirement satisfy", func() {
		co.WIVgprCount = 20
		co.WFSgprCount = 16
		co.WGGroupSegmentByteSize = 1024

		wg := grid.WorkGroups[0]
		req := timing.NewMapWGReq(nil, nil, 10, wg, status)

		ok := wgMapper.MapWG(req)

		Expect(ok).To(BeTrue())
		Expect(wgMapper.SGprMask.StatusCount(cu.AllocStatusFree)).To(
			Equal(118))
		Expect(wgMapper.SGprMask.StatusCount(cu.AllocStatusReserved)).To(
			Equal(10))
		Expect(wgMapper.LDSMask.StatusCount(cu.AllocStatusFree)).To(
			Equal(252))
		Expect(wgMapper.LDSMask.StatusCount(cu.AllocStatusReserved)).To(
			Equal(4))
		Expect(wgMapper.VGprMask[0].StatusCount(cu.AllocStatusFree)).To(
			Equal(49))
		Expect(wgMapper.VGprMask[0].StatusCount(cu.AllocStatusReserved)).To(
			Equal(15))
		Expect(wgMapper.VGprMask[1].StatusCount(cu.AllocStatusFree)).To(
			Equal(49))
		Expect(wgMapper.VGprMask[1].StatusCount(cu.AllocStatusReserved)).To(
			Equal(15))
		Expect(wgMapper.VGprMask[2].StatusCount(cu.AllocStatusFree)).To(
			Equal(54))
		Expect(wgMapper.VGprMask[2].StatusCount(cu.AllocStatusReserved)).To(
			Equal(10))
		Expect(wgMapper.VGprMask[3].StatusCount(cu.AllocStatusFree)).To(
			Equal(54))
		Expect(wgMapper.VGprMask[3].StatusCount(cu.AllocStatusReserved)).To(
			Equal(10))
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

})
