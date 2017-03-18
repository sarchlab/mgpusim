package emu

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// WfState represent what state a wf is in
type WfState int

const (
	fetching WfState = iota
	fetched
	running
	ready
)

// WfScheduleInfo stores the information associated with a wavefront in the
// scheduler
type WfScheduleInfo struct {
	Wf      *Wavefront
	Inst    *disasm.Instruction
	InstBuf []byte
	State   WfState
}

// A Scheduler defines which wavefront in a ComputeUnit should execute
//
// In the emulator, we do not define the scheduler as a Yaotsu component.
type Scheduler struct {
	CU      gcn3.ComputeUnit
	Decoder *disasm.Disassembler
	Wfs     []*WfScheduleInfo
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
	info.State = ready
	s.Wfs = append(s.Wfs, info)
}

// Schedule will initiate the events to fetch and run instructions
func (s *Scheduler) Schedule(now core.VTimeInSec) {
	for _, wf := range s.Wfs {
		switch wf.State {
		case ready:
			s.doFetch(wf, now)
		case fetched:
		// Do issue
		default:
			// Fo nothing
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

func (s *Scheduler) doDecodeAndIssue(wf *WfScheduleInfo) {
	inst, err := s.Decoder.Decode(wf.InstBuf)
	if err != nil {
		log.Panic(err)
	}
	wf.Inst = inst

	// TODO issue
}

// Fetched is called when the ComputeUnit receives the instruction fetching
// respond
func (s *Scheduler) Fetched(wf *WfScheduleInfo, buf []byte) {
	wf.InstBuf = buf
	wf.State = fetched
}
