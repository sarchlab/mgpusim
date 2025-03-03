package cu

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/insts"
	"github.com/sarchlab/mgpusim/v4/kernels"
	"github.com/sarchlab/mgpusim/v4/protocol"
	"github.com/sarchlab/mgpusim/v4/timing/wavefront"
)

type mockScheduler struct {
}

func (m *mockScheduler) Run() bool {
	return true
}

func (m *mockScheduler) Pause() {
}

func (m *mockScheduler) Resume() {
}

func (m *mockScheduler) Flush() {
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
	wg.Packet = packet
	wg.CodeObject = grid.CodeObject
	grid.WorkGroups = append(grid.WorkGroups, wg)

	wf := kernels.NewWavefront()
	wf.WG = wg
	wg.Wavefronts = append(wg.Wavefronts, wf)

	return grid
}

var _ = Describe("ComputeUnit", func() {
	var (
		mockCtrl         *gomock.Controller
		cu               *ComputeUnit
		engine           *MockEngine
		wfDispatcher     *MockWfDispatcher
		decoder          *mockDecoder
		toInstMem        *MockPort
		toScalarMem      *MockPort
		toVectorMem      *MockPort
		toACE            *MockPort
		toCP             *MockPort
		branchUnit       *MockSubComponent
		vectorMemDecoder *MockSubComponent
		vectorMemUnit    *MockSubComponent
		scalarDecoder    *MockSubComponent
		vectorDecoder    *MockSubComponent
		ldsDecoder       *MockSubComponent
		scalarUnit       *MockSubComponent
		simdUnit         *MockSubComponent
		ldsUnit          *MockSubComponent

		instMem *MockPort

		grid *kernels.Grid

		scheduler *mockScheduler
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		wfDispatcher = NewMockWfDispatcher(mockCtrl)
		decoder = new(mockDecoder)
		scheduler = new(mockScheduler)
		branchUnit = NewMockSubComponent(mockCtrl)
		vectorMemDecoder = NewMockSubComponent(mockCtrl)
		vectorMemUnit = NewMockSubComponent(mockCtrl)
		scalarDecoder = NewMockSubComponent(mockCtrl)
		vectorDecoder = NewMockSubComponent(mockCtrl)
		ldsDecoder = NewMockSubComponent(mockCtrl)
		scalarUnit = NewMockSubComponent(mockCtrl)
		simdUnit = NewMockSubComponent(mockCtrl)
		ldsUnit = NewMockSubComponent(mockCtrl)

		cu = NewComputeUnit("CU", engine)
		cu.WfDispatcher = wfDispatcher
		cu.Decoder = decoder
		cu.Freq = 1
		cu.SRegFile = NewSimpleRegisterFile(1024, 0)
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(4096, 64))
		cu.Scheduler = scheduler

		cu.BranchUnit = branchUnit
		cu.VectorMemDecoder = vectorMemDecoder
		cu.VectorMemUnit = vectorMemUnit
		cu.ScalarDecoder = scalarDecoder
		cu.VectorDecoder = vectorDecoder
		cu.LDSDecoder = ldsDecoder
		cu.ScalarUnit = scalarUnit
		cu.SIMDUnit = append(cu.SIMDUnit, simdUnit)

		cu.LDSUnit = ldsUnit

		for i := 0; i < 4; i++ {
			cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
		}

		toInstMem = NewMockPort(mockCtrl)
		toACE = NewMockPort(mockCtrl)
		toScalarMem = NewMockPort(mockCtrl)
		toVectorMem = NewMockPort(mockCtrl)
		cu.ToInstMem = toInstMem
		cu.ToACE = toACE
		cu.ToScalarMem = toScalarMem
		cu.ToVectorMem = toVectorMem

		instMem = NewMockPort(mockCtrl)
		cu.InstMem = instMem

		toCP = NewMockPort(mockCtrl)

		cu.ToCP = toCP

		grid = exampleGrid()

		toInstMem.EXPECT().AsRemote().AnyTimes()
		toACE.EXPECT().AsRemote().AnyTimes()
		toScalarMem.EXPECT().AsRemote().AnyTimes()
		toVectorMem.EXPECT().AsRemote().AnyTimes()
		instMem.EXPECT().AsRemote().AnyTimes()
		toCP.EXPECT().AsRemote().AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when processing MapWGReq", func() {
		var (
			req *protocol.MapWGReq
		)

		BeforeEach(func() {
			wg := grid.WorkGroups[0]
			wg.Wavefronts = make([]*kernels.Wavefront, 2)
			wg.Wavefronts[0] = kernels.NewWavefront()
			wg.Wavefronts[1] = kernels.NewWavefront()
			location1 := protocol.WfDispatchLocation{
				Wavefront:  wg.Wavefronts[0],
				SIMDID:     1,
				VGPROffset: 100,
				SGPROffset: 10,
				LDSOffset:  100,
			}
			location2 := protocol.WfDispatchLocation{
				Wavefront:  wg.Wavefronts[1],
				SIMDID:     2,
				VGPROffset: 200,
				SGPROffset: 200,
				LDSOffset:  200,
			}

			builder := protocol.MapWGReqBuilder{}.
				WithSrc("").
				WithDst(cu.ToACE.AsRemote()).
				WithWG(wg).
				AddWf(location1).
				AddWf(location2)
			req = builder.Build()

			toACE.EXPECT().RetrieveIncoming().Return(req)
		})

		It("should dispatch wavefront", func() {
			wfDispatcher.EXPECT().
				DispatchWf(gomock.Any(), req.Wavefronts[0])
			wfDispatcher.EXPECT().
				DispatchWf(gomock.Any(), req.Wavefronts[1])
			engine.EXPECT().Schedule(gomock.Any())
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11)).AnyTimes()

			cu.processInputFromACE()

			Expect(cu.WfPools[1].wfs).To(HaveLen(1))
			Expect(cu.WfPools[2].wfs).To(HaveLen(1))
		})
	})

	Context("when handling DataReady from ToInstMem Port", func() {
		var (
			wf        *wavefront.Wavefront
			dataReady *mem.DataReadyRsp
		)
		BeforeEach(func() {
			wf = new(wavefront.Wavefront)
			inst := wavefront.NewInst(nil)
			wf.SetDynamicInst(inst)
			wf.PC = 0x1000

			req := mem.ReadReqBuilder{}.
				WithSrc(cu.ToInstMem.AsRemote()).
				WithDst(instMem.AsRemote()).
				WithAddress(0x100).
				WithByteSize(64).
				Build()

			dataReady = mem.DataReadyRspBuilder{}.
				WithSrc(instMem.AsRemote()).
				WithDst(cu.ToInstMem.AsRemote()).
				WithRspTo(req.ID).
				WithData([]byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				}).
				Build()

			toInstMem.EXPECT().RetrieveIncoming().Return(dataReady)

			info := new(InstFetchReqInfo)
			info.Wavefront = wf
			info.Req = req
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)
		})

		It("should handle fetch return", func() {
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(10))

			madeProgress := cu.processInputFromInstMem()

			//Expect(wf.State).To(Equal(WfFetched))
			Expect(wf.LastFetchTime).To(BeNumerically("~", 10))
			Expect(wf.PC).To(Equal(uint64(0x1000)))
			Expect(cu.InFlightInstFetch).To(HaveLen(0))
			Expect(wf.InstBuffer).To(HaveLen(64))
			Expect(madeProgress).To(BeTrue())
		})
	})

	Context("should handle DataReady from ToScalarMem port", func() {
		var (
			wf *wavefront.Wavefront
		)

		BeforeEach(func() {
			rawWf := grid.WorkGroups[0].Wavefronts[0]
			wf = wavefront.NewWavefront(rawWf)
			wf.SRegOffset = 0
			wf.OutstandingScalarMemAccess = 1
		})

		It("should handle scalar data load return", func() {
			read := mem.ReadReqBuilder{}.
				WithSrc(cu.ToScalarMem.AsRemote()).
				WithAddress(0x100).
				WithByteSize(64).
				Build()

			info := new(ScalarMemAccessInfo)
			info.Inst = wavefront.NewInst(insts.NewInst())
			info.Wavefront = wf
			info.DstSGPR = insts.SReg(0)
			info.Req = read
			cu.InFlightScalarMemAccess = append(cu.InFlightScalarMemAccess, info)

			rsp := mem.DataReadyRspBuilder{}.
				WithRspTo(read.ID).
				WithData(insts.Uint32ToBytes(32)).
				Build()
			toScalarMem.EXPECT().RetrieveIncoming().Return(rsp)

			cu.processInputFromScalarMem()

			access := RegisterAccess{
				Reg:        insts.SReg(0),
				RegCount:   1,
				WaveOffset: 0,
				Data:       make([]byte, 4),
			}
			cu.SRegFile.Read(access)
			Expect(insts.BytesToUint32(access.Data)).To(Equal(uint32(32)))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			Expect(cu.InFlightScalarMemAccess).To(HaveLen(0))
		})
	})

	Context("should handle DataReady from ToVectorMem", func() {
		var (
			rawWf *kernels.Wavefront
			wf    *wavefront.Wavefront
			inst  *wavefront.Inst
			read  *mem.ReadReq
			info  VectorMemAccessInfo
		)

		BeforeEach(func() {
			rawWf = grid.WorkGroups[0].Wavefronts[0]
			inst = wavefront.NewInst(insts.NewInst())
			inst.FormatType = insts.FLAT
			wf = wavefront.NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.SetDynamicInst(inst)
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1
			wf.OutstandingScalarMemAccess = 1

			read = mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithByteSize(16).
				CanWaitForCoalesce().
				Build()

			info = VectorMemAccessInfo{}
			info.Read = read
			info.Wavefront = wf
			info.Inst = inst
			info.laneInfo = []vectorMemAccessLaneInfo{
				{0, insts.VReg(0), 1, 0},
				{1, insts.VReg(0), 1, 4},
				{2, insts.VReg(0), 1, 8},
				{3, insts.VReg(0), 1, 12},
			}
			cu.InFlightVectorMemAccess = append(
				cu.InFlightVectorMemAccess, info)

			dataReady := mem.DataReadyRspBuilder{}.
				WithRspTo(read.ID).
				WithData(make([]byte, 16)).
				Build()
			for i := 0; i < 4; i++ {
				copy(dataReady.Data[i*4:i*4+4], insts.Uint32ToBytes(uint32(i)))
			}
			toVectorMem.EXPECT().RetrieveIncoming().Return(dataReady)
		})

		It("should handle vector data load return, and the return is not the last one for an instruction", func() {
			cu.processInputFromVectorMem()

			for i := 0; i < 4; i++ {
				access := RegisterAccess{}
				access.RegCount = 1
				access.WaveOffset = 0
				access.LaneID = i
				access.Reg = insts.VReg(0)
				access.Data = make([]byte, access.RegCount*4)
				cu.VRegFile[0].Read(access)
				Expect(insts.BytesToUint32(access.Data)).To(Equal(uint32(i)))
			}

			Expect(wf.OutstandingVectorMemAccess).To(Equal(1))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(1))
			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
		})

		It("should handle vector data load return, and the return is the last one for an instruction", func() {
			read.CanWaitForCoalesce = false

			cu.processInputFromVectorMem()

			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			for i := 0; i < 4; i++ {
				access := RegisterAccess{}
				access.RegCount = 1
				access.WaveOffset = 0
				access.LaneID = i
				access.Reg = insts.VReg(0)
				access.Data = make([]byte, access.RegCount*4)
				cu.VRegFile[0].Read(access)
				Expect(insts.BytesToUint32(access.Data)).To(Equal(uint32(i)))
			}
		})
	})

	Context("handle write done respond from ToVectorMem port", func() {
		var (
			rawWf    *kernels.Wavefront
			inst     *wavefront.Inst
			wf       *wavefront.Wavefront
			info     VectorMemAccessInfo
			writeReq *mem.WriteReq
			doneRsp  *mem.WriteDoneRsp
		)

		BeforeEach(func() {
			rawWf = grid.WorkGroups[0].Wavefronts[0]
			inst = wavefront.NewInst(insts.NewInst())
			inst.FormatType = insts.FLAT
			wf = wavefront.NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.SetDynamicInst(inst)
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1
			wf.OutstandingScalarMemAccess = 1

			writeReq = mem.WriteReqBuilder{}.
				WithAddress(0x100).
				CanWaitForCoalesce().
				Build()

			info = VectorMemAccessInfo{}
			info.Wavefront = wf
			info.Inst = inst
			info.Write = writeReq
			cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, info)

			doneRsp = mem.WriteDoneRspBuilder{}.
				WithRspTo(writeReq.ID).
				Build()
			toVectorMem.EXPECT().RetrieveIncoming().Return(doneRsp)
		})

		It("should handle vector data store return and the return is not the last one from an instruction", func() {
			madeProgress := cu.processInputFromVectorMem()

			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
			Expect(madeProgress).To(BeTrue())
		})

		It("should handle vector data store return and the return is the last one from an instruction", func() {
			writeReq.CanWaitForCoalesce = false

			cu.processInputFromVectorMem()

			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
		})
	})

	Context("should handle flush request", func() {
		It("should handle a pipeline flush request from CU", func() {
			req := protocol.CUPipelineFlushReqBuilder{}.
				WithSrc("").
				WithDst(cu.ToCP.AsRemote()).
				Build()

			toCP.EXPECT().RetrieveIncoming().Return(req)

			cu.processInputFromCP()

			Expect(cu.inCPRequestProcessingStage).To(BeIdenticalTo(req))
			Expect(cu.isFlushing).To(BeTrue())
			Expect(cu.currentFlushReq).To(BeIdenticalTo(req))
		})

		It("should flush internal CU buffers", func() {
			info := new(InstFetchReqInfo)
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)

			scalarMemInfo := new(ScalarMemAccessInfo)
			cu.InFlightScalarMemAccess = append(cu.InFlightScalarMemAccess, scalarMemInfo)

			vectorMemInfo := VectorMemAccessInfo{}
			cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, vectorMemInfo)

			cu.flushCUBuffers()

			Expect(cu.InFlightInstFetch).To(BeNil())
			Expect(cu.InFlightVectorMemAccess).To(BeNil())
			Expect(cu.InFlightScalarMemAccess).To(BeNil())
		})

		It("should handle a restart request", func() {
			cu.isPaused = true

			req := protocol.CUPipelineRestartReqBuilder{}.
				WithSrc("").
				WithDst(cu.ToCP.AsRemote()).
				Build()

			toCP.EXPECT().RetrieveIncoming().Return(req)
			toCP.EXPECT().Send(gomock.Any())

			cu.processInputFromCP()
			Expect(cu.isPaused).To(BeTrue())
			Expect(cu.isSendingOutShadowBufferReqs).To(BeTrue())
		})

		It("should flush the full CU", func() {
			req := protocol.CUPipelineFlushReqBuilder{}.
				WithSrc("").
				WithDst(cu.ToCP.AsRemote()).
				Build()

			cu.currentFlushReq = req

			info := new(InstFetchReqInfo)
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)

			scalarMemInfo := new(ScalarMemAccessInfo)
			cu.InFlightScalarMemAccess = append(cu.InFlightScalarMemAccess, scalarMemInfo)

			vectorMemInfo := VectorMemAccessInfo{}
			cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, vectorMemInfo)

			branchUnit.EXPECT().Flush()
			scalarUnit.EXPECT().Flush()
			scalarDecoder.EXPECT().Flush()
			simdUnit.EXPECT().Flush()
			vectorDecoder.EXPECT().Flush()
			ldsUnit.EXPECT().Flush()
			ldsDecoder.EXPECT().Flush()
			vectorMemDecoder.EXPECT().Flush()
			vectorMemUnit.EXPECT().Flush()

			cu.flushPipeline()

			Expect(cu.InFlightInstFetch).To(BeNil())
			Expect(cu.InFlightVectorMemAccess).To(BeNil())
			Expect(cu.InFlightScalarMemAccess).To(BeNil())

			Expect(cu.shadowInFlightInstFetch).To(Not(BeNil()))
			Expect(cu.shadowInFlightVectorMemAccess).To(Not(BeNil()))
			Expect(cu.shadowInFlightScalarMemAccess).To(Not(BeNil()))

			Expect(cu.toSendToCP).NotTo(BeNil())
			Expect(cu.isFlushing).To(BeFalse())
			Expect(cu.isPaused).To(BeTrue())
		})

		It("should not restart a CU where there are shadow buffer reqs pending", func() {
			info := new(InstFetchReqInfo)
			req := mem.ReadReqBuilder{}.
				WithSrc(cu.ToInstMem.AsRemote()).
				WithDst(instMem.AsRemote()).
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			info.Req = req

			cu.shadowInFlightInstFetch = append(cu.InFlightInstFetch, info)

			scalarMemInfo := new(ScalarMemAccessInfo)
			scalarMemInfo.Req = req
			cu.shadowInFlightScalarMemAccess = append(cu.InFlightScalarMemAccess, scalarMemInfo)

			vectorMemInfo := VectorMemAccessInfo{}
			vectorMemInfo.Read = req
			cu.shadowInFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, vectorMemInfo)

			toInstMem.EXPECT().Send(gomock.Any())
			toVectorMem.EXPECT().Send(gomock.Any())
			toScalarMem.EXPECT().Send(gomock.Any())

			cu.checkShadowBuffers()
		})

		It("should restart a CU where there are  no shadow buffer reqs pending", func() {
			cu.shadowInFlightInstFetch = nil
			cu.shadowInFlightScalarMemAccess = nil
			cu.shadowInFlightVectorMemAccess = nil

			cu.checkShadowBuffers()

			Expect(cu.isPaused).To(BeFalse())
		})
	})
})
