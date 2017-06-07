package cu

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/mem"
)

func prepareGrid() *kernels.Grid {
	// Prepare a mock grid that is expanded
	grid := kernels.NewGrid()
	for i := 0; i < 5; i++ {
		wg := kernels.NewWorkGroup()
		grid.WorkGroups = append(grid.WorkGroups, wg)
		for j := 0; j < 10; j++ {
			wf := kernels.NewWavefront()
			wf.WG = wg
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
	}
	return grid
}

type mockWGMapper struct {
	OK         bool
	UnmappedWg *WorkGroup
}

func (m *mockWGMapper) MapWG(req *timing.MapWGReq) bool {
	return m.OK
}

func (m *mockWGMapper) UnmapWG(wg *WorkGroup) {
	m.UnmappedWg = wg
}

type mockWfDispatcher struct {
	OK bool
}

func (m *mockWfDispatcher) DispatchWf(evt *DispatchWfEvent) (bool, *Wavefront) {
	return m.OK, nil
}

type mockWfArbitor struct {
	wfsToReturn [][]*Wavefront
}

func newMockWfArbitor() *mockWfArbitor {
	a := new(mockWfArbitor)
	a.wfsToReturn = make([][]*Wavefront, 0)
	return a
}

func (m *mockWfArbitor) Arbitrate([]*WavefrontPool) []*Wavefront {
	if len(m.wfsToReturn) == 0 {
		return nil
	}
	wfs := m.wfsToReturn[0]
	m.wfsToReturn = m.wfsToReturn[1:]
	return wfs
}

type mockDecoder struct {
	Inst *insts.Inst
}

func (d *mockDecoder) Decode(buf []byte) (*insts.Inst, error) {
	return d.Inst, nil
}

var _ = Describe("Scheduler", func() {
	var (
		scheduler        *Scheduler
		connection       *core.MockConnection
		engine           *core.MockEngine
		wgMapper         *mockWGMapper
		wfDispatcher     *mockWfDispatcher
		fetchArbitor     *mockWfArbitor
		issueArbitor     *mockWfArbitor
		instMem          *core.MockComponent
		branchUnit       *core.MockComponent
		vectorMemDecoder *core.MockComponent
		scalarDecoder    *core.MockComponent
		vectorDecoder    *core.MockComponent
		ldsDecoder       *core.MockComponent
		decoder          *mockDecoder
		grid             *kernels.Grid
		status           *timing.KernelDispatchStatus
		co               *insts.HsaCo
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		wgMapper = new(mockWGMapper)
		wfDispatcher = new(mockWfDispatcher)
		fetchArbitor = newMockWfArbitor()
		issueArbitor = newMockWfArbitor()
		branchUnit = core.NewMockComponent("branchUnit")
		vectorMemDecoder = core.NewMockComponent("vectorMemDecoder")
		scalarDecoder = core.NewMockComponent("scalarDecodor")
		vectorDecoder = core.NewMockComponent("vectorDecoder")
		ldsDecoder = core.NewMockComponent("ldsDecoder")
		instMem = core.NewMockComponent("instMem")
		decoder = new(mockDecoder)
		scheduler = NewScheduler("scheduler", engine, wgMapper, wfDispatcher,
			fetchArbitor, issueArbitor, decoder)
		scheduler.Freq = 1
		scheduler.InstMem = instMem
		scheduler.BranchUnit = branchUnit
		scheduler.VectorMemDecoder = vectorMemDecoder
		scheduler.ScalarDecoder = scalarDecoder
		scheduler.VectorDecoder = vectorDecoder
		scheduler.LDSDecoder = ldsDecoder

		connection = core.NewMockConnection()
		core.PlugIn(scheduler, "ToDispatcher", connection)
		core.PlugIn(scheduler, "ToDecoders", connection)
		core.PlugIn(scheduler, "ToInstMem", connection)

		grid = prepareGrid()
		status = timing.NewKernelDispatchStatus()
		status.Grid = grid
		co = insts.NewHsaCo()
		status.CodeObject = co
	})

	Context("when processing MapWGReq", func() {
		It("should process MapWGReq", func() {
			wg := kernels.NewWorkGroup()
			req := timing.NewMapWGReq(nil, scheduler, 10, wg, co)

			scheduler.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		})
	})

	Context("when processing DispatchWfReq", func() {
		It("should schedule DispatchWfEvent", func() {
			wg := grid.WorkGroups[0]
			wf := wg.Wavefronts[0]
			info := new(timing.WfDispatchInfo)
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf, info, 6256)

			scheduler.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		})
	})

	Context("when processing InstCompletionReq", func() {
		It("should mark wavefront to ready", func() {
			wf := new(Wavefront)
			wf.State = WfRunning
			req := NewInstCompletionReq(nil, scheduler, 10, wf)

			scheduler.Recv(req)

			Expect(wf.State).To(Equal(WfReady))
		})
	})

	Context("when handling MapWGEvent", func() {
		It("should reply OK if wgMapper say OK", func() {
			req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
				co)

			wgMapper.OK = true
			connection.ExpectSend(req, nil)

			scheduler.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(scheduler.RunningWGs).NotTo(BeEmpty())
			Expect(req.Ok).To(BeTrue())
		})

		It("should reply not OK if wgMapper say not OK", func() {
			req := timing.NewMapWGReq(nil, scheduler, 10, grid.WorkGroups[0],
				co)

			wgMapper.OK = false
			connection.ExpectSend(req, nil)

			scheduler.Handle(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(req.Ok).To(BeFalse())
		})
	})

	Context("when handling DispatchWfEvent", func() {
		It("should reschedule DispatchWfEvent if not complete", func() {
			wf := grid.WorkGroups[0].Wavefronts[0]
			info := new(timing.WfDispatchInfo)
			info.SIMDID = 1
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf, info, 6256)
			evt := NewDispatchWfEvent(10, scheduler, req)

			wfDispatcher.OK = false
			scheduler.Handle(evt)

			Expect(len(engine.ScheduledEvent)).To(Equal(1))
		})

		It("should add wavefront to workgroup", func() {
			wf := grid.WorkGroups[0].Wavefronts[0]
			wf.WG = grid.WorkGroups[0]
			managedWG := NewWorkGroup(wf.WG, nil)
			info := new(timing.WfDispatchInfo)
			info.SIMDID = 1
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf, info, 6256)
			evt := NewDispatchWfEvent(10, scheduler, req)
			scheduler.RunningWGs[grid.WorkGroups[0]] = managedWG
			scheduler.running = false

			wfDispatcher.OK = true
			scheduler.Handle(evt)

			Expect(len(engine.ScheduledEvent)).To(Equal(1))
			Expect(len(managedWG.Wfs)).To(Equal(1))
		})

		It("should not schedule tick if scheduling is already running", func() {
			wf := grid.WorkGroups[0].Wavefronts[0]
			wf.WG = grid.WorkGroups[0]
			managedWG := NewWorkGroup(wf.WG, nil)
			info := new(timing.WfDispatchInfo)
			info.SIMDID = 1
			req := timing.NewDispatchWfReq(nil, scheduler, 10, wf, info, 6256)
			evt := NewDispatchWfEvent(10, scheduler, req)
			scheduler.RunningWGs[grid.WorkGroups[0]] = managedWG
			scheduler.running = true

			wfDispatcher.OK = true
			scheduler.Handle(evt)

			Expect(len(engine.ScheduledEvent)).To(Equal(0))
			Expect(len(managedWG.Wfs)).To(Equal(1))
		})
	})

	Context("when handling TickEvent", func() {
		It("should fetch", func() {
			wf := new(Wavefront)
			wf.PC = 8064
			fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
				[]*Wavefront{wf})

			reqToExpect := mem.NewAccessReq()
			reqToExpect.SetSrc(scheduler)
			reqToExpect.SetDst(instMem)
			reqToExpect.Address = 8064
			reqToExpect.ByteSize = 8
			reqToExpect.Type = mem.Read
			reqToExpect.SetSendTime(10)
			reqToExpect.Info = wf
			// connection.ExpectSend(reqToExpect, nil)

			scheduler.Handle(core.NewTickEvent(10, scheduler))

			Expect(len(engine.ScheduledEvent)).To(Equal(1))
			// Expect(connection.AllExpectedSent()).To(BeTrue())
			// Expect(wf.State).To(Equal(WfFetching))
		})

		It("should issue", func() {
			wfs := make([]*Wavefront, 0)
			issueDirs := []insts.ExeUnit{
				insts.ExeUnitBranch,
				insts.ExeUnitLDS,
				insts.ExeUnitVMem,
				insts.ExeUnitVALU,
				insts.ExeUnitScalar,
			}
			issueTo := []core.Component{
				branchUnit, ldsDecoder, vectorMemDecoder, vectorDecoder,
				scalarDecoder,
			}
			reqs := make([]core.Req, 0)

			for i := 0; i < 5; i++ {
				wf := new(Wavefront)
				wf.State = WfFetched
				wf.Inst = NewInst(insts.NewInst())
				wf.Inst.ExeUnit = issueDirs[i]
				wfs = append(wfs, wf)

				if issueTo[i] != nil {
					req := NewIssueInstReq(scheduler, issueTo[i], 10.5,
						scheduler, wf)
					reqs = append(reqs, req)
				}
			}

			issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)

			scheduler.Handle(core.NewTickEvent(10, scheduler))

			Expect(len(engine.ScheduledEvent)).To(Equal(5))
			for i := 0; i < 5; i++ {
				reqToSend := engine.ScheduledEvent[i].(*core.DeferredSend).Req
				Expect(core.ReqEquivalent(reqToSend, reqs[i])).To(BeTrue())
			}
		})

		It("should issue internal instruction", func() {
			wfs := make([]*Wavefront, 0)
			wf := new(Wavefront)
			wf.Inst = NewInst(insts.NewInst())
			wf.Inst.ExeUnit = insts.ExeUnitSpecial
			wf.State = WfFetched
			wfs = append(wfs, wf)

			issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)
			scheduler.internalExecuting = nil

			scheduler.Handle(core.NewTickEvent(10, scheduler))

			Expect(scheduler.internalExecuting).To(BeIdenticalTo(wf))
			Expect(wf.State).To(Equal(WfRunning))
		})

		// It("should not issue internal instruction, if there is one internal instruction in flight", func() {
		// 	wfs := make([]*Wavefront, 0)
		// 	wf := new(Wavefront)
		// 	wf.Inst = NewInst(insts.NewInst())
		// 	wf.Inst.ExeUnit = insts.ExeUnitSpecial
		// 	wf.State = WfFetched
		// 	wfs = append(wfs, wf)

		// 	issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)
		// 	scheduler.internalExecuting = new(Wavefront)
		// 	scheduler.internalExecuting.Inst = NewInst(insts.NewInst())

		// 	scheduler.Handle(core.NewTickEvent(10, scheduler))

		// 	Expect(scheduler.internalExecuting).NotTo(BeIdenticalTo(wf))
		// 	Expect(wf.State).To(Equal(WfFetched))
		// })

		It("should evaluate internal executing insts", func() {
			wf := new(Wavefront)
			wf.Inst = NewInst(insts.NewInst())
			wf.Inst.Format = insts.FormatTable[insts.Sopp]
			wf.Inst.Opcode = 1 // S_ENFPGM

			scheduler.internalExecuting = wf
			scheduler.Handle(core.NewTickEvent(10, scheduler))

			Expect(len(engine.ScheduledEvent)).To(Equal(1))
		})

	})

	Context("when processing the mem.AccessReq", func() {
		It("should set wavefront status", func() {
			wf := new(Wavefront)
			wf.Inst = NewInst(nil)
			wf.PC = 6604
			req := mem.NewAccessReq()
			req.Info = wf
			req.SetRecvTime(10)
			inst := insts.NewInst()
			inst.ByteSize = 4
			decoder.Inst = inst

			scheduler.Recv(req)

			Expect(wf.State).To(Equal(WfFetched))
			Expect(wf.LastFetchTime).To(Equal(core.VTimeInSec(10)))
			Expect(wf.Inst.Inst).To(BeIdenticalTo(inst))
			Expect(wf.PC).To(Equal(uint64(6608)))
		})
	})

	Context("when handling WfCompleteEvent", func() {
		It("should clear all the wg reservation and send a message back", func() {
			wg := grid.WorkGroups[0]
			mapReq := timing.NewMapWGReq(nil, scheduler, 0, wg, nil)
			mapReq.SwapSrcAndDst()
			managedWG := NewWorkGroup(wg, nil)
			managedWG.MapReq = mapReq
			scheduler.RunningWGs[wg] = managedWG

			var wfToComplete *Wavefront
			for i := 0; i < len(wg.Wavefronts); i++ {
				managedWf := new(Wavefront)
				managedWf.Wavefront = wg.Wavefronts[i]
				managedWf.State = WfCompleted
				managedWf.SIMDID = i % 4
				if i == 6 {
					managedWf.State = WfRunning
					wfToComplete = managedWf
				}
				managedWG.Wfs = append(managedWG.Wfs, managedWf)

				scheduler.WfPools[i%4].AddWf(managedWf)
			}

			evt := NewWfCompleteEvent(0, scheduler, wfToComplete)
			reqToSend := timing.NewWGFinishMesg(scheduler, nil, 0, wg)
			connection.ExpectSend(reqToSend, nil)

			scheduler.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(wgMapper.UnmappedWg).To(BeIdenticalTo(managedWG))
			for i := 0; i < 4; i++ {
				Expect(scheduler.WfPools[i].Availability()).To(Equal(10))
			}

		})
	})
})
