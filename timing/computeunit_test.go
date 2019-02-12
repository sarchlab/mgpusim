package timing

import (
	"log"
	"reflect"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
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

type mockScheduler struct {
}

func (m *mockWfDispatcher) DispatchWf(now akita.VTimeInSec, wf *Wavefront) {
}

func (m *mockScheduler) Run(now akita.VTimeInSec) bool {
	return true
}

func (m *mockScheduler) StartDraining() {
}

func (m *mockScheduler) StopDraining() {
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
		mockCtrl     *gomock.Controller
		cu           *ComputeUnit
		engine       *mock_akita.MockEngine
		wgMapper     *mockWGMapper
		wfDispatcher *mockWfDispatcher
		decoder      *mockDecoder
		toInstMem    *mock_akita.MockPort
		toScalarMem  *mock_akita.MockPort
		toVectorMem  *mock_akita.MockPort
		toACE        *mock_akita.MockPort
		toCP         *mock_akita.MockPort
		cp           *mock_akita.MockPort

		instMem *mock_akita.MockPort

		grid *kernels.Grid

		scheduler *mockScheduler
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = mock_akita.NewMockEngine(mockCtrl)
		wgMapper = new(mockWGMapper)
		wfDispatcher = new(mockWfDispatcher)
		decoder = new(mockDecoder)
		scheduler = new(mockScheduler)

		cu = NewComputeUnit("cu", engine)
		cu.WGMapper = wgMapper
		cu.WfDispatcher = wfDispatcher
		cu.Decoder = decoder
		cu.Freq = 1
		cu.SRegFile = NewSimpleRegisterFile(1024, 0)
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(4096, 64))
		cu.Scheduler = scheduler

		for i := 0; i < 4; i++ {
			cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
		}

		toInstMem = mock_akita.NewMockPort(mockCtrl)
		toACE = mock_akita.NewMockPort(mockCtrl)
		toScalarMem = mock_akita.NewMockPort(mockCtrl)
		toVectorMem = mock_akita.NewMockPort(mockCtrl)
		cu.ToInstMem = toInstMem
		cu.ToACE = toACE
		cu.ToScalarMem = toScalarMem
		cu.ToVectorMem = toVectorMem

		instMem = mock_akita.NewMockPort(mockCtrl)
		cu.InstMem = instMem

		toCP = mock_akita.NewMockPort(mockCtrl)
		cp = mock_akita.NewMockPort(mockCtrl)

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
			req.SetRecvTime(10)
			req.SetEventTime(10)

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
			wf        *Wavefront
			dataReady *mem.DataReadyRsp
		)
		BeforeEach(func() {
			wf = new(Wavefront)
			inst := NewInst(nil)
			wf.inst = inst
			wf.PC = 0x1000

			req := mem.NewReadReq(8, cu.ToInstMem, instMem, 0x100, 64)

			dataReady = mem.NewDataReadyRsp(10,
				instMem, cu.ToInstMem, req.ID)
			dataReady.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			dataReady.SetRecvTime(10)
			dataReady.SetEventTime(10)
			toInstMem.EXPECT().Retrieve(gomock.Any()).Return(dataReady)

			info := new(InstFetchReqInfo)
			info.Wavefront = wf
			info.Req = req
			cu.inFlightInstFetch = append(cu.inFlightInstFetch, info)
		})

		It("should handle fetch return", func() {
			cu.processInputFromInstMem(10)

			//Expect(wf.State).To(Equal(WfFetched))
			Expect(wf.LastFetchTime).To(BeNumerically("~", 10))
			Expect(wf.PC).To(Equal(uint64(0x1000)))
			Expect(cu.inFlightInstFetch).To(HaveLen(0))
			Expect(wf.InstBuffer).To(HaveLen(64))
			Expect(cu.NeedTick).To(BeTrue())
		})
	})

	Context("should handle DataReady from ToScalarMem port", func() {
		It("should handle scalar data load return", func() {
			rawWf := grid.WorkGroups[0].Wavefronts[0]
			wf := NewWavefront(rawWf)
			wf.SRegOffset = 0
			wf.OutstandingScalarMemAccess = 1

			read := mem.NewReadReq(8, cu.ToScalarMem, nil, 0x100, 4)

			info := new(ScalarMemAccessInfo)
			info.Wavefront = wf
			info.DstSGPR = insts.SReg(0)
			info.Req = read
			cu.inFlightScalarMemAccess = append(cu.inFlightScalarMemAccess, info)

			req := mem.NewDataReadyRsp(10, nil, nil, read.ID)
			req.Data = insts.Uint32ToBytes(32)
			req.SetSendTime(10)
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
			Expect(cu.inFlightScalarMemAccess).To(HaveLen(0))
		})
	})

	Context("should handle DataReady from ToVectorMem", func() {
		var (
			rawWf *kernels.Wavefront
			wf    *Wavefront
			inst  *Inst
			read  *mem.ReadReq
			info  *VectorMemAccessInfo
		)

		BeforeEach(func() {
			rawWf = grid.WorkGroups[0].Wavefronts[0]
			inst = NewInst(insts.NewInst())
			inst.FormatType = insts.FLAT
			wf = NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.inst = inst
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1
			wf.OutstandingScalarMemAccess = 1

			read = mem.NewReadReq(8, nil, nil, 0x100, 16)

			info = new(VectorMemAccessInfo)
			info.Read = read
			info.Wavefront = wf
			info.Inst = inst
			info.DstVGPR = insts.VReg(0)
			info.Lanes = []int{0, 1, 2, 3}
			info.LaneAddrOffsets = []uint64{0, 4, 8, 12}
			cu.inFlightVectorMemAccess = append(
				cu.inFlightVectorMemAccess, info)

			dataReady := mem.NewDataReadyRsp(10, nil, nil, read.ID)
			dataReady.Data = make([]byte, 64)
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
			Expect(cu.inFlightVectorMemAccess).To(HaveLen(0))
		})

		It("should handle vector data load return, and the return is the last one for an instruction", func() {
			read.IsLastInWave = true

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
			inst     *Inst
			wf       *Wavefront
			info     *VectorMemAccessInfo
			writeReq *mem.WriteReq
			doneRsp  *mem.DoneRsp
		)

		BeforeEach(func() {
			rawWf = grid.WorkGroups[0].Wavefronts[0]
			inst = NewInst(insts.NewInst())
			inst.FormatType = insts.FLAT
			wf = NewWavefront(rawWf)
			wf.SIMDID = 0
			wf.inst = inst
			wf.VRegOffset = 0
			wf.OutstandingVectorMemAccess = 1
			wf.OutstandingScalarMemAccess = 1

			writeReq = mem.NewWriteReq(8, nil, nil, 0x100)

			info = new(VectorMemAccessInfo)
			info.Wavefront = wf
			info.Inst = inst
			info.Write = writeReq
			cu.inFlightVectorMemAccess = append(cu.inFlightVectorMemAccess, info)

			doneRsp = mem.NewDoneRsp(10, nil, nil, writeReq.ID)
			toVectorMem.EXPECT().Retrieve(gomock.Any()).Return(doneRsp)
		})

		It("should handle vector data store return and the return is not the last one from an instruction", func() {
			cu.processInputFromVectorMem(10)

			Expect(cu.inFlightVectorMemAccess).To(HaveLen(0))
			Expect(cu.NeedTick).To(BeTrue())
		})

		It("should handle vector data store return and the return is the last one from an instruction", func() {
			writeReq.IsLastInWave = true

			cu.processInputFromVectorMem(10)

			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			Expect(cu.inFlightVectorMemAccess).To(HaveLen(0))
		})
	})
	Context("should process an input from CP", func() {
		It("handle a Pipeline drain request from CP", func() {
			req := gcn3.NewCUPipelineDrainReq(10, nil, cu.ToCP)
			req.SetEventTime(10)

			toCP.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(req)

			cu.processInputFromCP(11)

			Expect(cu.inCPRequestProcessingStage).To(BeIdenticalTo(req))
			Expect(cu.isDraining).To(BeTrue())

		})
	})

})

type ControlComponent struct {
	*akita.TickingComponent
	cu   akita.Port
	toCU akita.Port
}

type cuPipelineDrainReqEvent struct {
	*akita.EventBase
	req *gcn3.CUPipelineDrainReq
}

func newCUPipelineDrainReqEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	req *gcn3.CUPipelineDrainReq,
) *cuPipelineDrainReqEvent {
	return &cuPipelineDrainReqEvent{akita.NewEventBase(time, handler), req}
}

func NewControlComponent(
	name string,
	engine akita.Engine,
) *ControlComponent {
	ctrlComponent := new(ControlComponent)
	ctrlComponent.TickingComponent = akita.NewTickingComponent(name, engine, 1*akita.GHz, ctrlComponent)
	ctrlComponent.toCU = akita.NewLimitNumReqPort(ctrlComponent, 1)
	return ctrlComponent

}

func (ctrl *ControlComponent) Handle(e akita.Event) error {
	switch evt := e.(type) {
	case akita.TickEvent:
		ctrl.handleTickEvent(evt)
	case cuPipelineDrainReqEvent:
		ctrl.handleCUPipelineDrain(evt)
	default:
		log.Panicf("cannot handle handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (ctrlComp *ControlComponent) handleTickEvent(tick akita.TickEvent) {
	now := tick.Time()
	ctrlComp.NeedTick = false

	ctrlComp.parseFromCU(now)

	if ctrlComp.NeedTick {
		ctrlComp.TickLater(now)
	}

}

func (ctrlComp *ControlComponent) parseFromCU(now akita.VTimeInSec) {
	cuReq := ctrlComp.toCU.Retrieve(now)

	if cuReq == nil {
		return
	}

	switch req := cuReq.(type) {
	case gcn3.CUPipelineDrainRsp:
		ctrlComp.checkCU(now, req)
		return
	default:
		log.Panicf("Received an unsupported request type %s from CU \n", reflect.TypeOf(cuReq))
	}

}

func (ctrlComp *ControlComponent) checkCU(now akita.VTimeInSec, req akita.Req) {
	//How do we access the internal states without magic?
}

func (ctrlComp *ControlComponent) handleCUPipelineDrain(evt cuPipelineDrainReqEvent) {
	req := evt.req
	sendErr := ctrlComp.toCU.Send(req)
	if sendErr != nil {
		log.Panicf("Unable to send drain request to CU")
	}

}

//Design a new component that sends MAPWGReq to CU.
//It can handle a event so that it can send drain req
//It can receive CU drain complete
//After Receiving check if the CU is idle and check times
var _ = Describe("Compute unit black box", func() {
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

		ctrlComponent = new(ControlComponent)
		ctrlComponent.Engine = engine //Verify if same engine
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

	})

})
