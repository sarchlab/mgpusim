package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/mem"
)

type mockWGMapper struct {
	OK         bool
	UnmappedWg *WorkGroup
}

func (m *mockWGMapper) MapWG(req *gcn3.MapWGReq) bool {
	return m.OK
}

func (m *mockWGMapper) UnmapWG(wg *WorkGroup) {
	m.UnmappedWg = wg
}

type mockWfDispatcher struct {
}

func (m *mockWfDispatcher) DispatchWf(now core.VTimeInSec, wf *Wavefront) {
}

type mockDecoder struct {
	Inst *insts.Inst
}

func (d *mockDecoder) Decode(buf []byte) (*insts.Inst, error) {
	return d.Inst, nil
}

func exampleGrid() *kernels.Grid {
	grid := kernels.NewGrid()

	grid.CodeObject = insts.NewHsaCo()
	grid.CodeObject.HsaCoHeader = new(insts.HsaCoHeader)

	packet := new(kernels.HsaKernelDispatchPacket)
	grid.Packet = packet

	wg := kernels.NewWorkGroup()
	wg.Grid = grid
	grid.WorkGroups = append(grid.WorkGroups, wg)

	wf := kernels.NewWavefront()
	wf.WG = wg
	wg.Wavefronts = append(wg.Wavefronts, wf)

	return grid
}

var _ = Describe("ComputeUnit", func() {
	var (
		cu           *ComputeUnit
		engine       *core.MockEngine
		wgMapper     *mockWGMapper
		wfDispatcher *mockWfDispatcher
		decoder      *mockDecoder

		connection *core.MockConnection
		instMem    *core.MockComponent

		grid *kernels.Grid
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		wgMapper = new(mockWGMapper)
		wfDispatcher = new(mockWfDispatcher)
		decoder = new(mockDecoder)

		cu = NewComputeUnit("cu", engine)
		cu.WGMapper = wgMapper
		cu.WfDispatcher = wfDispatcher
		cu.Decoder = decoder
		cu.Freq = 1
		cu.SRegFile = NewSimpleRegisterFile(1024, 0)
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(4096, 64))

		for i := 0; i < 4; i++ {
			cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
		}

		connection = core.NewMockConnection()
		connection.PlugIn(cu.ToACE)

		instMem = core.NewMockComponent("InstMem")
		cu.InstMem = instMem.ToOutside

		grid = exampleGrid()
	})

	Context("when processing MapWGReq", func() {
		It("should schedule wavefront dispatching if mapping is successful", func() {
			wgMapper.OK = true

			wg := grid.WorkGroups[0]
			req := gcn3.NewMapWGReq(nil, cu.ToACE, 10, wg)
			req.SetRecvTime(10)
			req.SetEventTime(10)

			cu.Handle(req)

			Expect(engine.ScheduledEvent).To(HaveLen(2))
		})

		It("should schedule more events if number of wavefronts is greater than 4", func() {
			wgMapper.OK = true

			wg := grid.WorkGroups[0]
			wg.Wavefronts = make([]*kernels.Wavefront, 0)
			for i := 0; i < 6; i++ {
				wf := kernels.NewWavefront()
				wf.WG = wg
				wg.Wavefronts = append(wg.Wavefronts, wf)
			}
			req := gcn3.NewMapWGReq(nil, cu.ToACE, 10, wg)
			req.SetRecvTime(10)
			req.SetEventTime(10)

			cu.Handle(req)

			Expect(engine.ScheduledEvent).To(HaveLen(7))
		})

		It("should reply not OK if there are pending wavefronts", func() {
			wf := grid.WorkGroups[0].Wavefronts[0]
			cu.WfToDispatch[wf] = new(WfDispatchInfo)

			wg := grid.WorkGroups[0]
			req := gcn3.NewMapWGReq(nil, cu.ToACE, 10, wg)
			req.SetRecvTime(10)
			req.SetEventTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu.ToACE, nil, 10, wg)
			expectedResponse.Ok = false
			expectedResponse.SetSendTime(10)
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should reply not OK if mapping is failed", func() {
			wgMapper.OK = false

			wg := grid.WorkGroups[0]
			req := gcn3.NewMapWGReq(nil, cu.ToACE, 10, wg)
			req.SetRecvTime(10)
			req.SetEventTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu.ToACE, nil, 10, wg)
			expectedResponse.Ok = false
			expectedResponse.SetRecvTime(10)
			expectedResponse.SetSendTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})
	})

	Context("when handling WfDispatchEvent", func() {

	})

	Context("when handling mem.AccessReq", func() {
		It("should handle fetch return", func() {
			wf := new(Wavefront)
			inst := NewInst(nil)
			wf.inst = inst
			wf.PC = 0x1000

			req := mem.NewDataReadyRsp(10, instMem.ToOutside, cu.ToInstMem, "out_req")
			req.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			req.SetRecvTime(10)
			req.SetEventTime(10)

			info := new(MemAccessInfo)
			info.Action = MemAccessInstFetch
			info.Wf = wf
			cu.inFlightMemAccess["out_req"] = info

			cu.Handle(req)

			//Expect(wf.State).To(Equal(WfFetched))
			Expect(wf.LastFetchTime).To(BeNumerically("~", 10))
			Expect(wf.PC).To(Equal(uint64(0x1000)))
			Expect(cu.inFlightMemAccess).To(HaveLen(0))
			Expect(wf.InstBuffer).To(HaveLen(64))
		})

		It("should handle scalar data load return", func() {
			rawWf := grid.WorkGroups[0].Wavefronts[0]
			inst := NewInst(insts.NewInst())
			wf := NewWavefront(rawWf)
			wf.inst = inst
			wf.SRegOffset = 0
			wf.OutstandingScalarMemAccess = 1

			info := newMemAccessInfo()
			info.Action = MemAccessScalarDataLoad
			info.Wf = wf
			info.Dst = insts.SReg(0)
			cu.inFlightMemAccess["out_req"] = info

			req := mem.NewDataReadyRsp(10, nil, nil, "out_req")
			req.Data = insts.Uint32ToBytes(32)
			req.SetSendTime(10)

			cu.Handle(req)

			access := new(RegisterAccess)
			access.Reg = insts.SReg(0)
			access.WaveOffset = 0
			access.RegCount = 1
			cu.SRegFile.Read(access)
			Expect(insts.BytesToUint32(access.Data)).To(Equal(uint32(32)))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			Expect(cu.inFlightMemAccess).To(HaveLen(0))
		})

		It("should handle vector data load return, and the return is not the last one for an instruction", func() {
			rawWf := grid.WorkGroups[0].Wavefronts[0]
			inst := NewInst(insts.NewInst())
			wf := NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.inst = inst
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1

			info := newMemAccessInfo()
			info.Action = MemAccessVectorDataLoad
			info.Address = 4096
			info.Wf = wf
			info.TotalReqs = 4
			info.ReturnedReqs = 1
			info.Inst = inst
			info.Dst = insts.VReg(0)
			for i := 0; i < 64; i++ {
				info.PreCoalescedAddrs[i] = uint64(4096 + i*4)
			}
			cu.inFlightMemAccess["out_req"] = info

			req := mem.NewDataReadyRsp(10, nil, nil, "out_req")
			req.Data = make([]byte, 64)
			for i := 0; i < 16; i++ {
				copy(req.Data[i*4:i*4+4], insts.Uint32ToBytes(uint32(i)))
			}

			cu.Handle(req)

			Expect(info.ReturnedReqs).To(Equal(2))
			for i := 0; i < 16; i++ {
				access := new(RegisterAccess)
				access.RegCount = 1
				access.WaveOffset = 0
				access.LaneID = i
				access.Reg = insts.VReg(0)
				cu.VRegFile[0].Read(access)
				Expect(insts.BytesToUint32(access.Data)).To(Equal(uint32(i)))
			}
			Expect(cu.inFlightMemAccess).To(HaveLen(0))

		})

		It("should handle vector data load return, and the return is the last one for an instruction", func() {
			rawWf := grid.WorkGroups[0].Wavefronts[0]
			inst := NewInst(insts.NewInst())
			wf := NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.inst = inst
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1

			info := newMemAccessInfo()
			info.Action = MemAccessVectorDataLoad
			info.Wf = wf
			info.TotalReqs = 4
			info.ReturnedReqs = 3
			info.Inst = inst
			info.Dst = insts.VReg(0)
			info.Address = 4096 + 64*3
			for i := 0; i < 64; i++ {
				info.PreCoalescedAddrs[i] = uint64(4096 + i*4)
			}
			cu.inFlightMemAccess["out_req"] = info

			req := mem.NewDataReadyRsp(10, nil, nil, "out_req")
			req.Data = make([]byte, 64)
			for i := 0; i < 16; i++ {
				copy(req.Data[i*4:i*4+4], insts.Uint32ToBytes(uint32(i+48)))
			}

			cu.Handle(req)

			Expect(info.ReturnedReqs).To(Equal(4))
			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			for i := 48; i < 64; i++ {
				access := new(RegisterAccess)
				access.RegCount = 1
				access.WaveOffset = 0
				access.LaneID = i
				access.Reg = insts.VReg(0)
				cu.VRegFile[0].Read(access)
				Expect(insts.BytesToUint32(access.Data)).To(Equal(uint32(i)))
			}
		})

		It("should handle vector data store return and the return is not the last one from an instruction", func() {

			rawWf := grid.WorkGroups[0].Wavefronts[0]
			inst := NewInst(insts.NewInst())
			wf := NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.inst = inst
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1

			info := newMemAccessInfo()
			info.Action = MemAccessVectorDataStore
			info.Wf = wf
			info.TotalReqs = 4
			info.ReturnedReqs = 1
			info.Inst = inst
			info.Dst = insts.VReg(0)
			info.Address = 4096 + 64*3
			cu.inFlightMemAccess["out_req"] = info

			req := mem.NewDoneRsp(10, nil, nil, "out_req")

			cu.Handle(req)

			Expect(info.ReturnedReqs).To(Equal(2))
			Expect(cu.inFlightMemAccess).To(HaveLen(0))
		})

		It("should handle vector data store return and the return is the last one from an instruction", func() {

			rawWf := grid.WorkGroups[0].Wavefronts[0]
			inst := NewInst(insts.NewInst())
			wf := NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.inst = inst
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1

			info := newMemAccessInfo()
			info.Action = MemAccessVectorDataStore
			info.Wf = wf
			info.TotalReqs = 4
			info.ReturnedReqs = 3
			info.Inst = inst
			info.Dst = insts.VReg(0)
			info.Address = 4096 + 64*3
			cu.inFlightMemAccess["out_req"] = info

			req := mem.NewDoneRsp(10, nil, nil, "out_req")

			cu.Handle(req)

			Expect(info.ReturnedReqs).To(Equal(4))
			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			Expect(cu.inFlightMemAccess).To(HaveLen(0))
		})
	})

})
