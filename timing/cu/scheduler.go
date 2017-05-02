package cu

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/timing"
)

// A Scheduler is responsible for determine which wavefront can fetch, decode,
// and issue
//
//     ToDispatcher <=>  The port conneting the scheduler and the dispatcher
//     ToSReg <=> The port connecting the scheduler with the scalar register
// 				  file
//     ToVRegs <=> The port connecting ithe scheduler with the vector register
//                files
type Scheduler struct {
	*core.BasicComponent
	sync.Mutex

	engine   core.Engine
	wgMapper WGMapper
	SRegFile core.Component

	WfPools []*WavefrontPool

	used    bool
	Freq    core.Freq
	Running bool

	MappedWGs []*timing.MapWGReq
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(name string, engine core.Engine, wgMapper WGMapper) *Scheduler {
	s := new(Scheduler)
	s.engine = engine
	s.BasicComponent = core.NewBasicComponent(name)

	s.initWfPools([]int{10, 10, 10, 10})
	s.used = false

	s.wgMapper = wgMapper

	s.AddPort("ToDispatcher")
	s.AddPort("ToSReg")
	s.AddPort("ToVRegs")
	return s
}

func (s *Scheduler) initWfPools(numWfs []int) {
	s.WfPools = make([]*WavefrontPool, 0, len(numWfs))
	for i := 0; i < len(numWfs); i++ {
		s.WfPools = append(s.WfPools, NewWavefrontPool(numWfs[i]))
	}
}

// Recv function process the incoming requests
func (s *Scheduler) Recv(req core.Req) *core.Error {
	s.Lock()
	defer s.Unlock()

	switch req := req.(type) {
	case *timing.MapWGReq:
		return s.processMapWGReq(req)
	case *timing.DispatchWfReq:
		return s.processDispatchWfReq(req)
	default:
		log.Panicf("Unable to process req %s", reflect.TypeOf(req))
	}
	return nil
}

func (s *Scheduler) processMapWGReq(req *timing.MapWGReq) *core.Error {
	s.used = true
	evt := NewMapWGEvent(s, s.Freq.NextTick(req.RecvTime()), req)
	s.engine.Schedule(evt)
	return nil
}

func (s *Scheduler) processDispatchWfReq(
	req *timing.DispatchWfReq,
) *core.Error {
	evt := NewDispatchWfEvent(s, s.Freq.NextTick(req.RecvTime()), req)
	s.engine.Schedule(evt)
	return nil
}

// Handle processes the event that is scheduled on this scheduler
func (s *Scheduler) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *MapWGEvent:
		return s.handleMapWGEvent(evt)
	case *DispatchWfEvent:
		return s.handleDispatchWfEvent(evt)
	default:
		log.Panicf("Cannot handle event type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (s *Scheduler) handleMapWGEvent(evt *MapWGEvent) error {
	req := evt.Req

	ok := s.wgMapper.MapWG(req)

	req.Ok = ok
	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	s.GetConnection("ToDispatcher").Send(req)

	return nil
}

func (s *Scheduler) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	req := evt.Req
	wf := req.Wf
	info := req.Info

	wfPool := s.WfPools[info.SIMDID]
	managedWf := new(Wavefront)
	managedWf.Wavefront = wf
	managedWf.LDSOffset = info.LDSOffset
	managedWf.SRegOffset = info.SGPROffset
	managedWf.VRegOffset = info.VGPROffset
	wfPool.Wfs = append(wfPool.Wfs, managedWf)

	s.initWfRegs(managedWf, evt)

	if !s.Running {
		s.Running = true
		evt := NewScheduleEvent(s, s.Freq.NextTick(evt.Time()))
		s.engine.Schedule(evt)
	}

	return nil
}

func (s *Scheduler) initWfRegs(wf *Wavefront, evt *DispatchWfEvent) {
	req := evt.Req
	wf.PC = req.EntryPoint
	s.initSRegs(wf, evt)
	s.initVRegs(wf, evt)
}

func (s *Scheduler) initSRegs(wf *Wavefront, evt *DispatchWfEvent) {
	req := evt.Req
	co := req.Wf.WG.Grid.CodeObject
	packet := req.Wf.WG.Grid.Packet
	now := evt.Time()
	count := 0

	if co.EnableSgprPrivateSegmentBuffer() {
		log.Panic("Initializing register PrivateSegmentBuffer is not supported")
		count += 4
	}

	if co.EnableSgprDispatchPtr() {
		reg := insts.SReg(count)
		// FIXME: Fillin the correct value
		bytes := insts.Uint64ToBytes(0)
		s.writeReg(wf, reg, bytes, now)
		count += 2
	}

	if co.EnableSgprQueuePtr() {
		log.Println("Initializing register QueuePtr is not supported")
		count += 2
	}

	if co.EnableSgprKernelArgSegmentPtr() {
		reg := insts.SReg(count)
		bytes := insts.Uint64ToBytes(packet.KernargAddress)
		s.writeReg(wf, reg, bytes, now)
		count += 2
	}

	if co.EnableSgprDispatchId() {
		log.Println("Initializing register DispatchId is not supported")
		count += 2
	}

	if co.EnableSgprFlatScratchInit() {
		log.Println("Initializing register FlatScratchInit is not supported")
		count += 2
	}

	if co.EnableSgprPrivateSegementSize() {
		log.Println("Initializing register PrivateSegementSize is not supported")
		count++
	}

	if co.EnableSgprGridWorkGroupCountX() {
		log.Println("Initializing register GridWorkGroupCountX is not supported")
		count++
	}

	if co.EnableSgprGridWorkGroupCountY() {
		log.Println("Initializing register GridWorkGroupCountY is not supported")
		count++
	}

	if co.EnableSgprGridWorkGroupCountZ() {
		log.Println("Initializing register GridWorkGroupCountZ is not supported")
		count++
	}

	if co.EnableSgprWorkGroupIdX() {
		reg := insts.SReg(count)
		bytes := insts.Uint32ToBytes(uint32(wf.WG.IDX))
		s.writeReg(wf, reg, bytes, now)
		count++
	}

	if co.EnableSgprWorkGroupIdY() {
		reg := insts.SReg(count)
		bytes := insts.Uint32ToBytes(uint32(wf.WG.IDY))
		s.writeReg(wf, reg, bytes, now)
		count++
	}

	if co.EnableSgprWorkGroupIdZ() {
		reg := insts.SReg(count)
		bytes := insts.Uint32ToBytes(uint32(wf.WG.IDZ))
		s.writeReg(wf, reg, bytes, now)
		count++
	}

	if co.EnableSgprWorkGroupInfo() {
		log.Println("Initializing register GridWorkGroupInfo is not supported")
		count++
	}

	if co.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Println("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count++
	}
}

func (s *Scheduler) initVRegs(wf *Wavefront, evt *DispatchWfEvent) {
}

func (s *Scheduler) writeReg(
	wf *Wavefront,
	reg *insts.Reg,
	data []byte,
	now core.VTimeInSec,
) {
	if reg.IsSReg() {
		req := NewWriteRegReq(now, reg, wf.SRegOffset, data)
		req.SetSrc(s)
		req.SetDst(s.SRegFile)
		s.GetConnection("ToSReg").Send(req)
	} else {
		req := NewWriteRegReq(now, reg, wf.VRegOffset, data)
		req.SetSrc(s)
		req.SetDst(s.WfPools[wf.SIMDID].VRegFile)
		s.GetConnection("ToVRegs").Send(req)
	}
}

// ScheduleEvent requires the scheduler to schedule for the next cycle
type ScheduleEvent struct {
	*core.BasicEvent
}

// NewScheduleEvent returns a newly created ScheduleEvent
func NewScheduleEvent(
	handler core.Handler,
	time core.VTimeInSec,
) *ScheduleEvent {
	e := new(ScheduleEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	return e
}
