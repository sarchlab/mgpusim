package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
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

type fakeWfDispatcher struct {
	dispatched []protocol.WfDispatchLocation
}

func (d *fakeWfDispatcher) DispatchWf(
	wf *wavefront.Wavefront,
	location protocol.WfDispatchLocation,
) {
	d.dispatched = append(d.dispatched, location)
}

func exampleGrid() *kernels.Grid {
	grid := kernels.NewGrid()

	grid.CodeObject = &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	}

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
		cu               *ComputeUnit
		engine           *fakeEngine
		wfDispatcher     *fakeWfDispatcher
		decoder          *mockDecoder
		toInstMem        *fakePort
		toScalarMem      *fakePort
		toVectorMem      *fakePort
		toACE            *fakePort
		toCP             *fakePort
		branchUnit       *mockCUComponent
		vectorMemDecoder *mockCUComponent
		vectorMemUnit    *mockCUComponent
		scalarDecoder    *mockCUComponent
		vectorDecoder    *mockCUComponent
		ldsDecoder       *mockCUComponent
		scalarUnit       *mockCUComponent
		simdUnit         *mockCUComponent
		ldsUnit          *mockCUComponent

		grid *kernels.Grid

		scheduler *mockScheduler
	)

	BeforeEach(func() {
		engine = newFakeEngine()
		wfDispatcher = new(fakeWfDispatcher)
		decoder = new(mockDecoder)
		scheduler = new(mockScheduler)
		branchUnit = new(mockCUComponent)
		vectorMemDecoder = new(mockCUComponent)
		vectorMemUnit = new(mockCUComponent)
		scalarDecoder = new(mockCUComponent)
		vectorDecoder = new(mockCUComponent)
		ldsDecoder = new(mockCUComponent)
		scalarUnit = new(mockCUComponent)
		simdUnit = new(mockCUComponent)
		ldsUnit = new(mockCUComponent)

		cu = newTestComputeUnit("CU", engine)
		cu.WfDispatcher = wfDispatcher
		cu.Decoder = decoder
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

		toInstMem = newFakePort("CU.InstMem")
		toACE = newFakePort("CU.Top")
		toScalarMem = newFakePort("CU.ScalarMem")
		toVectorMem = newFakePort("CU.VectorMem")
		toCP = newFakePort("CU.Ctrl")
		cu.ToInstMem = toInstMem
		cu.ToACE = toACE
		cu.ToScalarMem = toScalarMem
		cu.ToVectorMem = toVectorMem
		cu.ToCP = toCP

		cu.comp.State.InstMem = "InstMem"
		cu.comp.State.ScalarMem = "ScalarMem"

		grid = exampleGrid()
	})

	Context("when processing MapWGReq", func() {
		var (
			req protocol.MapWGReq
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

			req = protocol.MapWGReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Dst: cu.ToACE.AsRemote(),
				},
				WorkGroup: wg,
				Wavefronts: []protocol.WfDispatchLocation{
					location1, location2,
				},
			}

			toACE.incoming = append(toACE.incoming, req)
		})

		It("should dispatch wavefront", func() {
			engine.now = 11

			cu.processInputFromACE()

			Expect(wfDispatcher.dispatched).To(HaveLen(2))
			Expect(wfDispatcher.dispatched[0]).To(Equal(req.Wavefronts[0]))
			Expect(wfDispatcher.dispatched[1]).To(Equal(req.Wavefronts[1]))
			Expect(cu.WfPools[1].wfs).To(HaveLen(1))
			Expect(cu.WfPools[2].wfs).To(HaveLen(1))
		})
	})

	Context("when handling DataReady from ToInstMem Port", func() {
		var (
			wf *wavefront.Wavefront
		)
		BeforeEach(func() {
			wf = new(wavefront.Wavefront)
			inst := wavefront.NewInst(nil)
			wf.SetDynamicInst(inst)
			wf.SetPC(0x1000)

			req := memprotocol.ReadReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: cu.ToInstMem.AsRemote(),
					Dst: cu.comp.State.InstMem,
				},
				Address:        0x100,
				AccessByteSize: 64,
			}

			dataReady := memprotocol.DataReadyRsp{
				MsgMeta: messaging.MsgMeta{
					ID:    timing.GetIDGenerator().Generate(),
					Src:   cu.comp.State.InstMem,
					Dst:   cu.ToInstMem.AsRemote(),
					RspTo: req.ID,
				},
				Data: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
			}

			toInstMem.incoming = append(toInstMem.incoming, dataReady)

			info := new(InstFetchReqInfo)
			info.Wavefront = wf
			info.Req = req
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)
		})

		It("should handle fetch return", func() {
			engine.now = 10

			madeProgress := cu.processInputFromInstMem()

			Expect(wf.LastFetchTime).To(Equal(timing.VTimeInPicoSec(10)))
			Expect(wf.PC()).To(Equal(uint64(0x1000)))
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
			read := memprotocol.ReadReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: cu.ToScalarMem.AsRemote(),
				},
				Address:        0x100,
				AccessByteSize: 64,
			}

			info := new(ScalarMemAccessInfo)
			info.Inst = wavefront.NewInst(insts.NewInst())
			info.Wavefront = wf
			info.DstSGPR = insts.SReg(0)
			info.Req = read
			cu.InFlightScalarMemAccess = append(
				cu.InFlightScalarMemAccess, info)

			rsp := memprotocol.DataReadyRsp{
				MsgMeta: messaging.MsgMeta{
					ID:    timing.GetIDGenerator().Generate(),
					RspTo: read.ID,
				},
				Data: insts.Uint32ToBytes(32),
			}
			toScalarMem.incoming = append(toScalarMem.incoming, rsp)

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
			read  *memprotocol.ReadReq
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

			read = &memprotocol.ReadReq{
				MsgMeta: messaging.MsgMeta{
					ID: timing.GetIDGenerator().Generate(),
				},
				Address:            0x100,
				AccessByteSize:     16,
				CanWaitForCoalesce: true,
			}

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

			dataReady := memprotocol.DataReadyRsp{
				MsgMeta: messaging.MsgMeta{
					ID:    timing.GetIDGenerator().Generate(),
					RspTo: read.ID,
				},
				Data: make([]byte, 16),
			}
			for i := 0; i < 4; i++ {
				copy(dataReady.Data[i*4:i*4+4],
					insts.Uint32ToBytes(uint32(i)))
			}
			toVectorMem.incoming = append(toVectorMem.incoming, dataReady)
		})

		It("should handle vector data load return, and the return is not "+
			"the last one for an instruction", func() {
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

		It("should handle vector data load return, and the return is the "+
			"last one for an instruction", func() {
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
			writeReq *memprotocol.WriteReq
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

			writeReq = &memprotocol.WriteReq{
				MsgMeta: messaging.MsgMeta{
					ID: timing.GetIDGenerator().Generate(),
				},
				Address:            0x100,
				CanWaitForCoalesce: true,
			}

			info = VectorMemAccessInfo{}
			info.Wavefront = wf
			info.Inst = inst
			info.Write = writeReq
			cu.InFlightVectorMemAccess = append(
				cu.InFlightVectorMemAccess, info)

			doneRsp := memprotocol.WriteDoneRsp{
				MsgMeta: messaging.MsgMeta{
					ID:    timing.GetIDGenerator().Generate(),
					RspTo: writeReq.ID,
				},
			}
			toVectorMem.incoming = append(toVectorMem.incoming, doneRsp)
		})

		It("should handle vector data store return and the return is not "+
			"the last one from an instruction", func() {
			madeProgress := cu.processInputFromVectorMem()

			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
			Expect(madeProgress).To(BeTrue())
		})

		It("should handle vector data store return and the return is the "+
			"last one from an instruction", func() {
			writeReq.CanWaitForCoalesce = false

			cu.processInputFromVectorMem()

			Expect(wf.OutstandingVectorMemAccess).To(Equal(0))
			Expect(wf.OutstandingScalarMemAccess).To(Equal(0))
			Expect(cu.InFlightVectorMemAccess).To(HaveLen(0))
		})
	})

	Context("should handle flush request", func() {
		It("should handle a pipeline flush request from CU", func() {
			req := protocol.CUPipelineFlushReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: "CP",
					Dst: cu.ToCP.AsRemote(),
				},
			}

			toCP.incoming = append(toCP.incoming, req)

			cu.processInputFromCP()

			Expect(cu.comp.State.IsFlushing).To(BeTrue())
			Expect(cu.comp.State.HasFlushReq).To(BeTrue())
			Expect(cu.comp.State.FlushReqID).To(Equal(req.ID))
			Expect(cu.comp.State.FlushReqSrc).To(Equal(req.Src))
			Expect(toCP.incoming).To(HaveLen(0))
		})

		It("should flush internal CU buffers", func() {
			info := new(InstFetchReqInfo)
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)

			scalarMemInfo := new(ScalarMemAccessInfo)
			cu.InFlightScalarMemAccess = append(
				cu.InFlightScalarMemAccess, scalarMemInfo)

			vectorMemInfo := VectorMemAccessInfo{}
			cu.InFlightVectorMemAccess = append(
				cu.InFlightVectorMemAccess, vectorMemInfo)

			cu.flushCUBuffers()

			Expect(cu.InFlightInstFetch).To(BeNil())
			Expect(cu.InFlightVectorMemAccess).To(BeNil())
			Expect(cu.InFlightScalarMemAccess).To(BeNil())
		})

		It("should handle a restart request", func() {
			cu.comp.State.IsPaused = true

			req := protocol.CUPipelineRestartReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: "CP",
					Dst: cu.ToCP.AsRemote(),
				},
			}

			toCP.incoming = append(toCP.incoming, req)

			cu.processInputFromCP()

			Expect(toCP.sent).To(HaveLen(1))
			Expect(toCP.sent[0]).To(
				BeAssignableToTypeOf(protocol.CUPipelineRestartRsp{}))
			Expect(cu.comp.State.IsPaused).To(BeTrue())
			Expect(cu.comp.State.IsSendingOutShadowBufferReqs).To(BeTrue())
		})

		It("should flush the full CU", func() {
			req := protocol.CUPipelineFlushReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: "CP",
					Dst: cu.ToCP.AsRemote(),
				},
			}

			cu.comp.State.HasFlushReq = true
			cu.comp.State.FlushReqID = req.ID
			cu.comp.State.FlushReqSrc = req.Src

			info := new(InstFetchReqInfo)
			cu.InFlightInstFetch = append(cu.InFlightInstFetch, info)

			scalarMemInfo := new(ScalarMemAccessInfo)
			cu.InFlightScalarMemAccess = append(
				cu.InFlightScalarMemAccess, scalarMemInfo)

			vectorMemInfo := VectorMemAccessInfo{}
			cu.InFlightVectorMemAccess = append(
				cu.InFlightVectorMemAccess, vectorMemInfo)

			cu.flushPipeline()

			Expect(cu.InFlightInstFetch).To(BeNil())
			Expect(cu.InFlightVectorMemAccess).To(BeNil())
			Expect(cu.InFlightScalarMemAccess).To(BeNil())

			Expect(cu.shadowInFlightInstFetch).To(Not(BeNil()))
			Expect(cu.shadowInFlightVectorMemAccess).To(Not(BeNil()))
			Expect(cu.shadowInFlightScalarMemAccess).To(Not(BeNil()))

			Expect(branchUnit.flushed).To(BeTrue())
			Expect(scalarUnit.flushed).To(BeTrue())
			Expect(scalarDecoder.flushed).To(BeTrue())
			Expect(simdUnit.flushed).To(BeTrue())
			Expect(vectorDecoder.flushed).To(BeTrue())
			Expect(ldsUnit.flushed).To(BeTrue())
			Expect(ldsDecoder.flushed).To(BeTrue())
			Expect(vectorMemDecoder.flushed).To(BeTrue())
			Expect(vectorMemUnit.flushed).To(BeTrue())

			Expect(cu.comp.State.HasPendingCPRsp).To(BeTrue())
			Expect(cu.comp.State.IsFlushing).To(BeFalse())
			Expect(cu.comp.State.IsPaused).To(BeTrue())
		})

		It("should not restart a CU where there are shadow buffer reqs "+
			"pending", func() {
			req := memprotocol.ReadReq{
				MsgMeta: messaging.MsgMeta{
					ID:  timing.GetIDGenerator().Generate(),
					Src: cu.ToInstMem.AsRemote(),
					Dst: cu.comp.State.InstMem,
				},
				Address:        0x100,
				AccessByteSize: 64,
			}

			info := new(InstFetchReqInfo)
			info.Req = req
			cu.shadowInFlightInstFetch = append(
				cu.shadowInFlightInstFetch, info)

			scalarMemInfo := new(ScalarMemAccessInfo)
			scalarMemInfo.Req = req
			cu.shadowInFlightScalarMemAccess = append(
				cu.shadowInFlightScalarMemAccess, scalarMemInfo)

			vectorMemInfo := VectorMemAccessInfo{}
			readCopy := req
			vectorMemInfo.Read = &readCopy
			cu.shadowInFlightVectorMemAccess = append(
				cu.shadowInFlightVectorMemAccess, vectorMemInfo)

			cu.checkShadowBuffers()

			Expect(toInstMem.sent).To(HaveLen(1))
			Expect(toScalarMem.sent).To(HaveLen(1))
			Expect(toVectorMem.sent).To(HaveLen(1))
		})

		It("should restart a CU where there are no shadow buffer reqs "+
			"pending", func() {
			cu.shadowInFlightInstFetch = nil
			cu.shadowInFlightScalarMemAccess = nil
			cu.shadowInFlightVectorMemAccess = nil

			cu.checkShadowBuffers()

			Expect(cu.comp.State.IsPaused).To(BeFalse())
		})
	})
})
