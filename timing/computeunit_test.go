package timing

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/timing/mock_timing"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
)

type mockWGMapper struct {
	OK         bool
	UnmappedWg wavefront.WorkGroup
}

func (m *mockWGMapper) MapWG(req *gcn3.MapWGReq) bool {
	return m.OK
}

func (m *mockWGMapper) UnmapWG(wg *wavefront.WorkGroup) {
	m.UnmappedWg = *wg
}

type mockWfDispatcher struct {
}

type mockScheduler struct {
}

func (m *mockWfDispatcher) DispatchWf(now akita.VTimeInSec, wf *wavefront.Wavefront) {
}

func (m *mockScheduler) Run(now akita.VTimeInSec) bool {
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

type mockComponent struct {
}

func (comp *mockComponent) CanAcceptWave() bool {
	return true
}

func (comp *mockComponent) AcceptWave(wave wavefront.Wavefront, now akita.VTimeInSec) {

}

func (comp *mockComponent) Run(now akita.VTimeInSec) {

}

func (comp *mockComponent) IsIdle() bool {
	return true
}

func (comp *mockComponent) Flush() {

}

func exampleGrid() *kernels.Grid {
	grid := kernels.NewGrid()

	grid.CodeObject = insts.NewHsaCo()
	grid.CodeObject.HsaCoHeader = new(insts.HsaCoHeader)

	packet := new(kernels.HsaKernelDispatchPacket)
	grid.Packet = packet

	wg := kernels.NewWorkGroup()
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
		wgMapper         *mockWGMapper
		wfDispatcher     *mockWfDispatcher
		decoder          *mockDecoder
		toInstMem        *MockPort
		toScalarMem      *MockPort
		toVectorMem      *MockPort
		toACE            *MockPort
		toCP             *MockPort
		cp               *MockPort
		branchUnit       *mock_timing.MockCUComponent
		vectorMemDecoder *mock_timing.MockCUComponent
		vectorMemUnit    *mock_timing.MockCUComponent
		scalarDecoder    *mock_timing.MockCUComponent
		vectorDecoder    *mock_timing.MockCUComponent
		ldsDecoder       *mock_timing.MockCUComponent
		scalarUnit       *mock_timing.MockCUComponent
		simdUnit         *mock_timing.MockCUComponent
		ldsUnit          *mock_timing.MockCUComponent

		instMem *MockPort

		grid *kernels.Grid

		scheduler *mockScheduler
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		wgMapper = new(mockWGMapper)
		wfDispatcher = new(mockWfDispatcher)
		decoder = new(mockDecoder)
		scheduler = new(mockScheduler)
		branchUnit = mock_timing.NewMockCUComponent(mockCtrl)
		vectorMemDecoder = mock_timing.NewMockCUComponent(mockCtrl)
		vectorMemUnit = mock_timing.NewMockCUComponent(mockCtrl)
		scalarDecoder = mock_timing.NewMockCUComponent(mockCtrl)
		vectorDecoder = mock_timing.NewMockCUComponent(mockCtrl)
		ldsDecoder = mock_timing.NewMockCUComponent(mockCtrl)
		scalarUnit = mock_timing.NewMockCUComponent(mockCtrl)
		simdUnit = mock_timing.NewMockCUComponent(mockCtrl)
		ldsUnit = mock_timing.NewMockCUComponent(mockCtrl)

		cu = NewComputeUnit("cu", engine)
		cu.WGMapper = wgMapper
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
		cp = NewMockPort(mockCtrl)

		cu.ToCP = toCP
		cu.CP = cp

		grid = exampleGrid()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("when processing MapWGReq", func() {
		var (
			req *gcn3.MapWGReq
		)

		BeforeEach(func() {
			wg := grid.WorkGroups[0]
			req = gcn3.NewMapWGReq(nil, cu.ToACE, 10, wg)
			req.RecvTime = 10
			req.EventTime = 10

			toACE.EXPECT().Retrieve(gomock.Any()).Return(req)
		})

		It("should schedule wavefront dispatching if mapping is successful",
			func() {
				wgMapper.OK = true

				engine.EXPECT().
					Schedule(gomock.AssignableToTypeOf(&WfDispatchEvent{}))
				engine.EXPECT().
					Schedule(gomock.AssignableToTypeOf(&WfDispatchEvent{}))

				cu.processInputFromACE(11)
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
				WithSendTime(8).
				WithSrc(cu.ToInstMem).
				WithDst(instMem).
				WithAddress(0x100).
				WithByteSize(64).
				Build()

			dataReady = mem.DataReadyRspBuilder{}.
				WithSendTime(10).
				WithSrc(instMem).
				WithDst(cu.ToInstMem).
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

			dataReady.RecvTime = 10
			dataReady.EventTime = 10
			toInstMem.EXPECT().Retrieve(gomock.Any()).Return(dataReady)

			info := new(InstFetchReqInfo)
			info.Wavefront = wf
			info.Req = req
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)
		})

		It("should handle fetch return", func() {
			cu.processInputFromInstMem(10)

			//Expect(wf.State).To(Equal(WfFetched))
			Expect(wf.LastFetchTime).To(BeNumerically("~", 10))
			Expect(wf.PC).To(Equal(uint64(0x1000)))
			Expect(cu.InFlightInstFetch).To(HaveLen(0))
			Expect(wf.InstBuffer).To(HaveLen(64))
			Expect(cu.NeedTick).To(BeTrue())
		})
	})

	Context("should handle DataReady from ToScalarMem port", func() {
		It("should handle scalar data load return", func() {
			rawWf := grid.WorkGroups[0].Wavefronts[0]
			wf := wavefront.NewWavefront(rawWf)
			wf.SRegOffset = 0
			wf.OutstandingScalarMemAccess = 1

			read := mem.ReadReqBuilder{}.
				WithSendTime(8).
				WithSrc(cu.ToScalarMem).
				WithAddress(0x100).
				WithByteSize(64).
				Build()

			info := new(ScalarMemAccessInfo)
			info.Inst = wavefront.NewInst(insts.NewInst())
			info.Wavefront = wf
			info.DstSGPR = insts.SReg(0)
			info.Req = read
			cu.InFlightScalarMemAccess = append(cu.InFlightScalarMemAccess, info)

			req := mem.DataReadyRspBuilder{}.
				WithSendTime(10).
				WithRspTo(read.ID).
				WithData(insts.Uint32ToBytes(32)).
				Build()
			req.RecvTime = 10
			toScalarMem.EXPECT().Retrieve(gomock.Any()).Return(req)

			cu.processInputFromScalarMem(10)

			access := RegisterAccess{}
			access.Reg = insts.SReg(0)
			access.WaveOffset = 0
			access.RegCount = 1
			access.Data = make([]byte, 4)
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
				WithSendTime(8).
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
				WithSendTime(10).
				WithRspTo(read.ID).
				WithData(make([]byte, 16)).
				Build()
			for i := 0; i < 4; i++ {
				copy(dataReady.Data[i*4:i*4+4], insts.Uint32ToBytes(uint32(i)))
			}
			toVectorMem.EXPECT().Retrieve(gomock.Any()).Return(dataReady)
		})

		It("should handle vector data load return, and the return is not the last one for an instruction", func() {
			cu.processInputFromVectorMem(10)

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

			cu.processInputFromVectorMem(10)

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
				WithSendTime(8).
				WithAddress(0x100).
				CanWaitForCoalesce().
				Build()

			info = VectorMemAccessInfo{}
			info.Wavefront = wf
			info.Inst = inst
			info.Write = writeReq
			cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, info)

			doneRsp = mem.WriteDoneRspBuilder{}.
				WithSendTime(10).
				WithRspTo(writeReq.ID).
				Build()
			toVectorMem.EXPECT().Retrieve(gomock.Any()).Return(doneRsp)
		})

		It("should handle vector data store return and the return is not the last one from an instruction", func() {
			cu.processInputFromVectorMem(10)

			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
			Expect(cu.NeedTick).To(BeTrue())
		})

		It("should handle vector data store return and the return is the last one from an instruction", func() {
			writeReq.CanWaitForCoalesce = false

			cu.processInputFromVectorMem(10)

			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
		})
	})
	Context("should handle flush and drain requests", func() {
		It("handle a Pipeline drain request from CP", func() {
			req := gcn3.NewCUPipelineDrainReq(10, nil, cu.ToCP)
			req.EventTime = 10

			toCP.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			cu.processInputFromCP(11)

			Expect(cu.inCPRequestProcessingStage).To(BeIdenticalTo(req))
			Expect(cu.isDraining).To(BeTrue())

		})
		It("should handle a pipeline flush request from CU", func() {
			req := gcn3.NewCUPipelineFlushReq(10, nil, cu.ToCP)
			req.EventTime = 10

			toCP.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			cu.processInputFromCP(11)

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

		It("should restart a paused CU", func() {
			cu.isPaused = true

			rsp := gcn3.NewCUPipelineRestartReq(10, nil, cu.ToCP)
			rsp.RecvTime = 10
			rsp.EventTime = 10

			toCP.EXPECT().Retrieve(gomock.Any()).Return(rsp)

			cu.processInputFromCP(11)
			Expect(cu.isPaused).To(BeFalse())

		})

		It("should flush the full CU", func() {
			req := gcn3.NewCUPipelineFlushReq(10, nil, cu.ToCP)
			req.EventTime = 10

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

			cu.flushCycleLeft = 0
			cu.flushPipeline(10)

			Expect(cu.InFlightInstFetch).To(BeNil())
			Expect(cu.InFlightVectorMemAccess).To(BeNil())
			Expect(cu.InFlightScalarMemAccess).To(BeNil())

			Expect(cu.toSendToCP).NotTo(BeNil())
			Expect(cu.isFlushing).To(BeFalse())
			Expect(cu.isPaused).To(BeTrue())

		})

	})

})

func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

//Design a new component that sends MAPWGReq to CU.
//It can handle a event so that it can send drain req
//It can receive CU drain complete
//After Receiving check if the CU is idle and check times
/*var _ = Describe("Compute unit black box", func() {
	var (
		cu            *ComputeUnit
		connection    *akita.DirectConnection
		engine        akita.Engine
		memory        *mem.IdealMemController //Set to instmem
		ctrlComponent *ControlComponent
	)

	BeforeEach(func() {
		engine = akita.NewSerialEngine()
		cuBuilder := timing.NewBuilder()
		cuBuilder.CUName = "cu"
		cuBuilder.Engine = engine

		//CU inst mem and scalar mem
		memory = mem.NewIdealMemController("memory", engine, 4*mem.GB)
		memory.Latency = 300
		memory.Freq = 1

		cuBuilder.Decoder = insts.NewDisassembler()

		connection = akita.NewDirectConnection(engine)
		connection.PlugIn(cu.ToInstMem)
		connection.PlugIn(cu.ToScalarMem)
		connection.PlugIn(cu.ToVectorMem)

		cu.InstMem = memory.ToTop
		cu.ScalarMem = memory.ToTop

		lowModuleFinderForCU := new(cache.SingleLowModuleFinder)
		lowModuleFinderForCU.LowModule = memory.ToTop
		cu.VectorMemModules = lowModuleFinderForCU

		ctrlComponent = NewControlComponent("ctrl", engine)
		ctrlComponent.toCU = cu.CP
		ctrlComponent.cu = cu.ToCP

	})

	It("should start a benchmark. After some time when the drain request is received it should do a pipeline drain", func() {
		type KernelArgs struct {
			Output              driver.GPUPtr
			Filter              driver.GPUPtr
			Input               driver.GPUPtr
			History             driver.GPUPtr
			NumTaps             uint32
			Padding             uint32
			HiddenGlobalOffsetX int64
			HiddenGlobalOffsetY int64
			HiddenGlobalOffsetZ int64
		}

		type Benchmark struct {
			driver *driver.Driver
			hsaco  *insts.HsaCo

			Length       int
			numTaps      int
			inputData    []float32
			filterData   []float32
			gFilterData  driver.GPUPtr
			gHistoryData driver.GPUPtr
			gInputData   driver.GPUPtr
			gOutputData  driver.GPUPtr
		}

		b := new(Benchmark)
		hsacoBytes, err := fir.Asset("kernels.hsaco")
		if err != nil {
			log.Panic(err)
		}
		b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "FIR")

		kernArg := KernelArgs{
			b.gOutputData,
			b.gFilterData,
			b.gInputData,
			b.gHistoryData,
			uint32(b.numTaps),
			0,
			0, 0, 0,
		}

		gridSize := [3]uint32{uint32(b.Length), 0, 0}
		wgSize := [3]uint32{256, 1, 1}

		co := b.hsaco
		//packet :=

		xLeft := gridSize[0]
		yLeft := gridSize[1]
		zLeft := gridSize[2]

		wgIDX := 0
		wgIDY := 0
		wgIDZ := 0
		for zLeft > 0 {
			zToAllocate := min(zLeft, uint32(wgSize[2]))
			for yLeft > 0 {
				yToAllocate := min(yLeft, uint32(wgSize[1]))
				for xLeft > 0 {
					xToAllocate := min(xLeft, uint32(wgSize[0]))
					wg := kernels.NewWorkGroup()
					wg.Grid = g
					wg.CurrSizeX = int(xToAllocate)
					wg.CurrSizeY = int(yToAllocate)
					wg.CurrSizeZ = int(zToAllocate)
					wg.SizeX = int(g.Packet.WorkgroupSizeX)
					wg.SizeY = int(g.Packet.WorkgroupSizeY)
					wg.SizeZ = int(g.Packet.WorkgroupSizeZ)
					wg.IDX = wgIDX
					wg.IDY = wgIDY
					wg.IDZ = wgIDZ
					xLeft -= xToAllocate
					b.spawnWorkItems(wg)
					b.formWavefronts(wg)
					g.WorkGroups = append(g.WorkGroups, wg)
					wgIDX++
				}
				wgIDX = 0
				yLeft -= yToAllocate
				xLeft = g.Packet.GridSizeX
				wgIDY++
			}
			wgIDY = 0
			zLeft -= zToAllocate
			yLeft = g.Packet.GridSizeY
			wgIDZ++
		}

		//Grid builder builds a grid and spawns wgs
		//It needs a hsaco insts.Hsaco and a packet *HsaKernelDispatchPacket

		//How to retrieve wgs from the kernel

	})

})*/
