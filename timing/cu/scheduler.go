package cu

import (
	"log"
	"reflect"
	"sync"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

// A Scheduler is responsible for determine which wavefront can fetch, decode,
// and issue
//
//    <=> ToDispatcher The port conneting the scheduler and the dispatcher
//
type Scheduler struct {
	*core.BasicComponent
	sync.Mutex

	engine core.Engine

	NumWfPool       int
	WfPools         []*WavefrontPool
	WfPoolFreeCount []int

	used    bool
	Freq    core.Freq
	Running bool

	MappedWGs []*timing.MapWGReq

	SGprCount       int
	SGprGranularity int
	SGprMask        *ResourceMask

	VGprCount       []int
	VGprGranularity int
	VGprMask        []*ResourceMask

	LDSByteSize    int
	LDSGranularity int
	LDSMask        *ResourceMask
}

// NewScheduler creates and returns a new Scheduler
func NewScheduler(name string, engine core.Engine) *Scheduler {
	s := new(Scheduler)
	s.engine = engine
	s.BasicComponent = core.NewBasicComponent(name)

	s.initWfPools(4, []int{10, 10, 10, 10})
	s.initLDSInfo(64 * 1024) // 64K
	s.initSGPRInfo(2048)
	s.initVGPRInfo([]int{16384, 16384, 16384, 16384})

	s.used = false

	s.AddPort("ToDispatcher")
	return s
}

func (s *Scheduler) initWfPools(numWfPool int, numWfs []int) {
	s.NumWfPool = numWfPool
	s.WfPools = make([]*WavefrontPool, 0, s.NumWfPool)
	s.WfPoolFreeCount = make([]int, s.NumWfPool)
	for i := 0; i < s.NumWfPool; i++ {
		s.WfPools = append(s.WfPools, NewWavefrontPool(numWfs[i]))
		s.WfPoolFreeCount[i] = numWfs[i]
	}
}

func (s *Scheduler) initSGPRInfo(count int) {
	s.SGprCount = count
	s.SGprGranularity = 16
	s.SGprMask = NewResourceMask(s.SGprCount / s.SGprGranularity)
}

func (s *Scheduler) initLDSInfo(byteSize int) {
	s.LDSByteSize = byteSize
	s.LDSGranularity = 256
	s.LDSMask = NewResourceMask(s.LDSByteSize / s.LDSGranularity)
}

func (s *Scheduler) initVGPRInfo(count []int) {
	s.VGprCount = count
	s.VGprGranularity = 64 * 4 // 64 lanes, 4 register minimum allocation
	s.VGprMask = make([]*ResourceMask, 0, s.NumWfPool)
	for i := 0; i < s.NumWfPool; i++ {
		s.VGprMask = append(s.VGprMask,
			NewResourceMask(s.VGprCount[i]/s.VGprGranularity))
	}
}

// SetWfPoolSize changes the number of wavefront that the scheduler can handle
// The first argument is the number of wavefront pools, which should always
// match the number of SIMDs that the comput unit has. The second argument is
// a slice indicating the number of wavefronts that each wavefront pool can
// hold. This function must be called before the scheduler has been used,
// otherwise it will panic.
func (s *Scheduler) SetWfPoolSize(numWfPool int, numWfs []int) {
	if s.used {
		log.Panic("Scheduler cannot resize after mapped with a work-group")
	}

	s.initWfPools(numWfPool, numWfs)
	s.initVGPRInfo(append(s.VGprCount, s.VGprCount[0]))
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
	ok := true

	for _, wf := range req.WG.Wavefronts {
		req.WfDispatchMap[wf] = new(timing.WfDispatchInfo)
	}

	if !s.withinSGPRLimitation(req) || !s.withinLDSLimitation(req) {
		ok = false
	}

	if ok && !s.matchWfWithSIMDs(req) {
		ok = false
	}

	if ok {
		s.reserveResources(req)
	}

	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	req.Ok = ok
	s.GetConnection("ToDispatcher").Send(req)
	return nil
}

func (s *Scheduler) withinSGPRLimitation(req *timing.MapWGReq) bool {
	required := int(req.KernelStatus.CodeObject.WFSgprCount) / s.SGprGranularity
	for _, wf := range req.WG.Wavefronts {
		offset, ok := s.SGprMask.NextRegion(required, AllocStatusFree)
		if !ok {
			return false
		}
		req.WfDispatchMap[wf].SGPROffset = offset * 64 // 16 reg, 4 byte each
		s.SGprMask.SetStatus(offset, required, AllocStatusToReserve)
	}
	return true
}

func (s *Scheduler) withinLDSLimitation(req *timing.MapWGReq) bool {
	required := int(req.KernelStatus.CodeObject.WGGroupSegmentByteSize) /
		s.LDSGranularity
	offset, ok := s.LDSMask.NextRegion(required, AllocStatusFree)
	if !ok {
		return false
	}

	// Set the information
	for _, wf := range req.WG.Wavefronts {
		req.WfDispatchMap[wf].LDSOffset = offset * 256
	}
	s.LDSMask.SetStatus(offset, required, AllocStatusToReserve)
	return true
}

// Maps the wfs of a workgroup to the SIMDs in the compute unit
// This function sets the value of req.WfDispatchMap, to keep the information
// about which SIMD should a wf dispatch to. This function also returns
// a boolean value for if the matching is successful.
func (s *Scheduler) matchWfWithSIMDs(req *timing.MapWGReq) bool {
	nextSIMD := 0
	vgprToUse := make([]int, s.NumWfPool)
	wfPoolEntryUsed := make([]int, s.NumWfPool)

	for i := 0; i < len(req.WG.Wavefronts); i++ {
		firstSIMDTested := nextSIMD
		firstTry := true
		found := false
		required := int(req.KernelStatus.CodeObject.WIVgprCount) * 64 /
			s.VGprGranularity
		for firstTry || nextSIMD != firstSIMDTested {
			firstTry = false
			offset, ok := s.VGprMask[nextSIMD].NextRegion(required, AllocStatusFree)

			if ok && s.WfPoolFreeCount[nextSIMD]-wfPoolEntryUsed[nextSIMD] > 0 {
				found = true
				vgprToUse[nextSIMD] += required
				wfPoolEntryUsed[nextSIMD]++
				req.WfDispatchMap[req.WG.Wavefronts[i]].SIMDID = nextSIMD
				req.WfDispatchMap[req.WG.Wavefronts[i]].VGPROffset =
					offset * 4 * 64 * 4 // 4 regs per group, 64 lanes, 4 bytes
				s.VGprMask[nextSIMD].SetStatus(offset, required,
					AllocStatusToReserve)
			}
			nextSIMD++
			if nextSIMD >= s.NumWfPool {
				nextSIMD = 0
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (s *Scheduler) reserveResources(req *timing.MapWGReq) {
	for _, info := range req.WfDispatchMap {
		s.WfPoolFreeCount[info.SIMDID]--
	}

	s.SGprMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	s.LDSMask.ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	for i := 0; i < s.NumWfPool; i++ {
		s.VGprMask[i].ConvertStatus(AllocStatusToReserve, AllocStatusReserved)
	}
}

func (s *Scheduler) handleDispatchWfEvent(evt *DispatchWfEvent) error {
	req := evt.Req
	wf := req.Wf
	info := req.Info

	wfPool := s.WfPools[info.SIMDID]
	managedWf := new(Wavefront)
	managedWf.Wavefront = wf
	wfPool.Wfs = append(wfPool.Wfs, managedWf)

	s.initWfRegs(managedWf, req)

	if !s.Running {
		s.Running = true
		evt := NewScheduleEvent(s, s.Freq.NextTick(evt.Time()))
		s.engine.Schedule(evt)
	}

	return nil
}

func (s *Scheduler) allocateWfRegs(wf *Wavefront, req *timing.DispatchWfReq) {
}

func (s *Scheduler) initWfRegs(wf *Wavefront, req *timing.DispatchWfReq) {
	wf.PC = req.EntryPoint
}

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront

	PC          uint64
	FetchBuffer []byte
	SRegOffset  int
	VRegOffset  int
}

// MapWGEvent requres the Scheduler to reserve space for a workgroup.
// The workgroup will not run immediately. The dispatcher will wait for the
// scheduler to dispatch wavefronts to it.
type MapWGEvent struct {
	*core.BasicEvent

	Req *timing.MapWGReq
}

// NewMapWGEvent creates a new MapWGEvent
func NewMapWGEvent(
	handler core.Handler,
	time core.VTimeInSec,
	req *timing.MapWGReq,
) *MapWGEvent {
	e := new(MapWGEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	e.Req = req
	return e
}

// DispatchWfEvent requires the scheduler shart to schedule for the event.
type DispatchWfEvent struct {
	*core.BasicEvent

	Req *timing.DispatchWfReq
}

// NewDispatchWfEvent returns a newly created DispatchWfEvent
func NewDispatchWfEvent(
	handler core.Handler,
	time core.VTimeInSec,
	req *timing.DispatchWfReq,
) *DispatchWfEvent {
	e := new(DispatchWfEvent)
	e.BasicEvent = core.NewBasicEvent()
	e.SetHandler(handler)
	e.SetTime(time)
	e.Req = req
	return e
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
