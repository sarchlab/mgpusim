package sm

import (
	"github.com/sarchlab/accelsimtracing/message"
	"github.com/sarchlab/accelsimtracing/nvidia"
	"github.com/sarchlab/accelsimtracing/subcore"
	"github.com/sarchlab/akita/v3/sim"
)

type SM struct {
	*sim.TickingComponent

	ID string

	// meta
	toGPU       sim.Port
	toGPURemote sim.Port

	toSubcores   sim.Port
	subcores     map[string]*subcore.Subcore
	freeSubcores []*subcore.Subcore

	undispatchedWarps    []*nvidia.Warp
	unfinishedWarpsCount int64

	finishedThreadblocksCount int64
}

func (s *SM) SetGPURemotePort(remote sim.Port) {
	s.toGPURemote = remote
}

func (s *SM) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = s.reportFinishedKernels(now) || madeProgress
	madeProgress = s.dispatchThreadblocksToSubcores(now) || madeProgress
	madeProgress = s.processGPUInput(now) || madeProgress
	madeProgress = s.processSubcoresInput(now) || madeProgress

	// fmt.Println("SM tick, madeProgress:", madeProgress)

	return madeProgress
}

func (s *SM) processGPUInput(now sim.VTimeInSec) bool {
	msg := s.toGPU.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToSMMsg:
		s.processSMMsg(msg, now)
	default:
		panic("Unhandled message type")
	}

	return true
}

func (s *SM) processSubcoresInput(now sim.VTimeInSec) bool {
	msg := s.toSubcores.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SubcoreToSMMsg:
		s.processSubcoreSubcoresg(msg, now)
		return true
	default:
		panic("Unhandled message type")
	}
}

func (s *SM) processSMMsg(msg *message.DeviceToSMMsg, now sim.VTimeInSec) {
	for _, warp := range msg.Threadblock.Warps {
		s.undispatchedWarps = append(s.undispatchedWarps, &warp)
		s.unfinishedWarpsCount++
	}

	s.toGPU.Retrieve(now)
}

func (s *SM) processSubcoreSubcoresg(msg *message.SubcoreToSMMsg, now sim.VTimeInSec) {
	if msg.WarpFinished {
		s.freeSubcores = append(s.freeSubcores, s.subcores[msg.SubcoreID])
		s.unfinishedWarpsCount--
		if s.unfinishedWarpsCount == 0 {
			s.finishedThreadblocksCount++
		}
	}

	s.toSubcores.Retrieve(now)
}

func (s *SM) reportFinishedKernels(now sim.VTimeInSec) bool {
	if s.finishedThreadblocksCount == 0 {
		return false
	}

	msg := &message.SMToDeviceMsg{
		ThreadblockFinished: true,
		SMID:                s.ID,
	}
	msg.Src = s.toGPU
	msg.Dst = s.toGPURemote
	msg.SendTime = now

	err := s.toGPU.Send(msg)
	if err != nil {
		return false
	}

	s.finishedThreadblocksCount--

	return true
}

func (s *SM) dispatchThreadblocksToSubcores(now sim.VTimeInSec) bool {
	if len(s.freeSubcores) == 0 || len(s.undispatchedWarps) == 0 {
		return false
	}

	subcore := s.freeSubcores[0]
	warp := s.undispatchedWarps[0]

	msg := &message.SMToSubcoreMsg{
		Warp: *warp,
	}
	msg.Src = s.toSubcores
	msg.Dst = subcore.GetPortByName("ToSM")
	msg.SendTime = now

	err := s.toSubcores.Send(msg)
	if err != nil {
		return false
	}

	s.freeSubcores = s.freeSubcores[1:]
	s.undispatchedWarps = s.undispatchedWarps[1:]

	return false
}
