package subcore

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/accelsimtracing/message"
	"github.com/sarchlab/akita/v3/sim"
)

type Subcore struct {
	*sim.TickingComponent

	// meta
	toGPU       sim.Port
	toGPURemote sim.Port

	warp          benchmark.Warp
	nextInstToRun int64

	needMoreWarps bool
}

func (s *Subcore) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = s.requestMoreWarp(now) || madeProgress
	madeProgress = s.runWarp() || madeProgress
	madeProgress = s.processInput(now) || madeProgress

	return madeProgress
}

func (s *Subcore) processInput(now sim.VTimeInSec) bool {
	msg := s.toGPU.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToSubcoreMsg:
		s.processDeviceMsg(msg, now)
	default:
		panic("Unrecognized message")
	}

	return true
}

func (s *Subcore) processDeviceMsg(msg *message.DeviceToSubcoreMsg, now sim.VTimeInSec) {
	s.warp = msg.Warp

	s.nextInstToRun = 0

	s.toGPU.Retrieve(now)
}

func (s *Subcore) runWarp() bool {
	if s.nextInstToRun == s.warp.InstructionsCount {
		return false
	}

	s.nextInstToRun++
	if s.nextInstToRun == s.warp.InstructionsCount {
		s.needMoreWarps = true
	}

	return true
}

func (s *Subcore) requestMoreWarp(now sim.VTimeInSec) bool {
	if !s.needMoreWarps {
		return false
	}

	msg := &message.SubcoreToDeviceMsg{}
	msg.Src = s.toGPU
	msg.Dst = s.toGPURemote
	msg.SendTime = now

	err := s.toGPURemote.Send(msg)
	if err != nil {
		return false
	}

	s.needMoreWarps = false
	return true
}
