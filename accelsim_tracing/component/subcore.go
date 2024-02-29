package component

import "github.com/sarchlab/akita/v3/sim"

type Subcore struct {
	*sim.TickingComponent

	// meta
	toGPUSrc sim.Port
	toGPUDst sim.Port

	warp          Warp
	nextInstToRun int64

	needMoreWarps bool
}

func NewSubcore(
	name string,
	engine sim.Engine,
	freq sim.Freq,
	toGPUDst sim.Port,
) *Subcore {
	s := &Subcore{
		toGPUDst: toGPUDst,
	}
	s.TickingComponent = sim.NewTickingComponent(name, engine, freq, s)
	s.toGPUSrc = sim.NewLimitNumMsgPort(s, 4, "ToGPU")

	conn := sim.NewDirectConnection("Conn", engine, freq)
	conn.PlugIn(s.toGPUSrc, 1)
	conn.PlugIn(toGPUDst, 1)
	return s
}

func (s *Subcore) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = s.requestMoreWarp(now) || madeProgress
	madeProgress = s.runWarp() || madeProgress
	madeProgress = s.processInput(now) || madeProgress

	return madeProgress
}

func (s *Subcore) processInput(now sim.VTimeInSec) bool {
	msg := s.toGPUSrc.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *DeviceToSubcoreMsg:
		s.processDeviceMsg(msg, now)
	default:
		panic("Unrecognized message")
	}

	return true
}

func (s *Subcore) processDeviceMsg(msg *DeviceToSubcoreMsg, now sim.VTimeInSec) {
	s.warp = msg.warp

	s.nextInstToRun = 0

	s.toGPUSrc.Retrieve(now)
}

func (s *Subcore) runWarp() bool {
	if s.nextInstToRun == s.warp.InstructionsCount() {
		return false
	}

	s.nextInstToRun++
	if s.nextInstToRun == s.warp.InstructionsCount() {
		s.needMoreWarps = true
	}

	return true
}

func (s *Subcore) requestMoreWarp(now sim.VTimeInSec) bool {
	if !s.needMoreWarps {
		return false
	}

	msg := &SubcoreToDeviceMsg{}
	msg.Src = s.toGPUSrc
	msg.Dst = s.toGPUDst
	msg.SendTime = now

	err := s.toGPUSrc.Send(msg)
	if err != nil {
		return false
	}

	s.needMoreWarps = false
	return true
}
