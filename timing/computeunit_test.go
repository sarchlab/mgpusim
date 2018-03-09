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
	dispatchedWf *gcn3.DispatchWfReq
}

func (m *mockWfDispatcher) DispatchWf(wf *Wavefront, req *gcn3.DispatchWfReq) {
	m.dispatchedWf = req
}

type mockDecoder struct {
	Inst *insts.Inst
}

func (d *mockDecoder) Decode(buf []byte) (*insts.Inst, error) {
	return d.Inst, nil
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

		for i := 0; i < 4; i++ {
			cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
		}

		connection = core.NewMockConnection()
		core.PlugIn(cu, "ToACE", connection)

		instMem = core.NewMockComponent("InstMem")
		cu.InstMem = instMem
	})

	Context("when processing MapWGReq", func() {
		It("should reply OK if mapping is successful", func() {
			wgMapper.OK = true

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg)
			expectedResponse.Ok = true
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should reply not OK if there are pending wavefronts", func() {
			wf := kernels.NewWavefront()
			cu.WfToDispatch[wf] = new(WfDispatchInfo)

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg)
			expectedResponse.Ok = false
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should reply not OK if mapping is failed", func() {
			wgMapper.OK = false

			wg := kernels.NewWorkGroup()
			req := gcn3.NewMapWGReq(nil, cu, 10, wg)
			req.SetRecvTime(10)

			expectedResponse := gcn3.NewMapWGReq(cu, nil, 10, wg)
			expectedResponse.Ok = false
			expectedResponse.SetRecvTime(10)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})
	})

	Context("when processing DispatchWfReq", func() {
		It("should dispatch wf", func() {
			packet := new(kernels.HsaKernelDispatchPacket)
			co := new(insts.HsaCo)
			grid := kernels.NewGrid()
			grid.Packet = packet
			grid.CodeObject = co
			wg := kernels.NewWorkGroup()
			wg.Grid = grid
			cu.wrapWG(wg, nil)

			wf := kernels.NewWavefront()
			wf.WG = wg
			req := gcn3.NewDispatchWfReq(nil, cu, 10, wf)
			req.SetRecvTime(11)

			cu.Handle(req)

			Expect(wfDispatcher.dispatchedWf).To(BeIdenticalTo(req))
		})

		It("should handle WfDispatchCompletionEvent", func() {
			cu.running = false
			wf := kernels.NewWavefront()
			managedWf := new(Wavefront)
			managedWf.Wavefront = wf
			managedWf.State = WfDispatching

			info := new(WfDispatchInfo)
			info.Wavefront = wf
			info.SIMDID = 0
			cu.WfToDispatch[wf] = info

			req := gcn3.NewDispatchWfReq(nil, cu, 10, wf)
			evt := NewWfDispatchCompletionEvent(11, cu, managedWf)
			evt.DispatchWfReq = req

			expectedResponse := gcn3.NewDispatchWfReq(cu, nil, 11, wf)
			expectedResponse.SetSendTime(11)
			connection.ExpectSend(expectedResponse, nil)

			cu.Handle(evt)

			Expect(len(engine.ScheduledEvent)).To(Equal(1))
			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(len(cu.WfPools[0].wfs)).To(Equal(1))
			Expect(len(cu.WfToDispatch)).To(Equal(0))
			Expect(managedWf.State).To(Equal(WfReady))
			Expect(cu.running).To(BeTrue())
		})
	})

	Context("when handling mem.AccessReq", func() {
		It("should handle fetch return", func() {
			wf := new(Wavefront)
			inst := NewInst(nil)
			wf.inst = inst
			wf.PC = 0x1000

			req := mem.NewAccessReq()
			req.SetSrc(instMem)
			req.SetDst(cu)
			req.SetRecvTime(10)
			req.Type = mem.Read
			req.Info = &MemAccessInfo{MemAccessInstFetch, wf}
			req.ByteSize = 4

			rawInst := insts.NewInst()
			decoder.Inst = rawInst
			decoder.Inst.ByteSize = 4

			cu.Handle(req)

			Expect(wf.State).To(Equal(WfFetched))
			Expect(wf.LastFetchTime).To(BeNumerically("~", 10))
			Expect(wf.PC).To(Equal(uint64(0x1004)))
			Expect(wf.inst).To(BeIdenticalTo(inst))
			Expect(wf.inst.Inst).To(BeIdenticalTo(rawInst))
		})
	})

})
