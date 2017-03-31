package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// WfState represent what state a wf is in
type WfState int

// The Wavefront states
const (
	Fetching WfState = iota
	Fetched
	Decoded
	Running
	Ready
)

// WfScheduleInfo stores the information associated with a wavefront in the
// scheduler
type WfScheduleInfo struct {
	Wf      *Wavefront
	Inst    *disasm.Instruction
	InstBuf []byte
	State   WfState
}

// ScheduleEvent asks the compute unit to schedule
type ScheduleEvent struct {
	*core.BasicEvent
}

// NewScheduleEvent creates a new ScheduleEvent
func NewScheduleEvent() *ScheduleEvent {
	e := new(ScheduleEvent)
	e.BasicEvent = core.NewBasicEvent()
	return e
}

// A Scheduler defines which wavefront in a ComputeUnit should execute
//
// In the emulator, we do not define the scheduler as a Yaotsu component.
type Scheduler struct {
	CU         gcn3.ComputeUnit
	Decoder    Decoder
	InstWorker *InstWorker
	Wfs        []*WfScheduleInfo
}

// NewScheduler returns a new scheduler
func NewScheduler() *Scheduler {
	s := new(Scheduler)
	s.Wfs = make([]*WfScheduleInfo, 0)
	return s
}

// AddWf registers a wavefront that needs to be scheduled
func (s *Scheduler) AddWf(wf *Wavefront) {
	info := new(WfScheduleInfo)
	info.Wf = wf
	info.State = Ready
	s.Wfs = append(s.Wfs, info)
}

// Schedule will initiate the events to fetch and run instructions
func (s *Scheduler) Schedule(now core.VTimeInSec) {
	for _, wf := range s.Wfs {
		switch wf.State {
		case Ready:
			s.doFetch(wf, now)
		case Fetched:
			s.doDecode(wf, now)
		case Decoded:
			s.doIssue(wf, now)
		case Running:
			// Do nothing, wait for the instruction to finish
		default:
			log.Panic("unknown wf state")
		}
	}
}

func (s *Scheduler) doFetch(wf *WfScheduleInfo, now core.VTimeInSec) {
	log.Println(wf)
	info := &MemAccessInfo{true, wf}
	addr := disasm.BytesToUint64(s.CU.ReadReg(disasm.Regs[disasm.Pc],
		wf.Wf.FirstWiFlatID, 8))
	s.CU.ReadInstMem(addr, 8, info, now)
}

func (s *Scheduler) doDecode(wf *WfScheduleInfo, now core.VTimeInSec) {
	inst, err := s.Decoder.Decode(wf.InstBuf)
	if err != nil {
		log.Panic(err)
	}
	wf.Inst = inst
	wf.State = Decoded
}

func (s *Scheduler) doIssue(wf *WfScheduleInfo, now core.VTimeInSec) {

}

// Fetched is called when the ComputeUnit receives the instruction fetching
// respond
func (s *Scheduler) Fetched(wf *WfScheduleInfo, buf []byte) {
	wf.InstBuf = buf
	wf.State = Fetched
}

// Completed is used for the instruction worker to notify that the instruction
// is completed and the scheduler can schedule another instruction from the
// wavefront
func (s *Scheduler) Completed(wf *WfScheduleInfo) {
	wf.State = Ready
}
