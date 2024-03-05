package gpu

import (
	"github.com/sarchlab/accelsimtracing/message"
	"github.com/sarchlab/accelsimtracing/nvidia"
	"github.com/sarchlab/accelsimtracing/sm"
	"github.com/sarchlab/akita/v3/sim"
)

type GPU struct {
	*sim.TickingComponent

	ID string

	// meta
	toDriver       sim.Port
	toDriverRemote sim.Port

	toSMs   sim.Port
	sms     map[string]*sm.SM
	freeSMs []*sm.SM

	undispatchedThreadblocks    []*nvidia.Threadblock
	unfinishedThreadblocksCount int64

	finishedKernelsCount int64
}

func (g *GPU) SetDriverRemotePort(remote sim.Port) {
	g.toDriverRemote = remote
}

func (g *GPU) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = g.reportFinishedKernels(now) || madeProgress
	madeProgress = g.dispatchThreadblocksToSMs(now) || madeProgress
	madeProgress = g.processDriverInput(now) || madeProgress
	madeProgress = g.processSMsInput(now) || madeProgress

	// fmt.Println("GPU tick, madeProgress:", madeProgress)

	return madeProgress
}

func (g *GPU) processDriverInput(now sim.VTimeInSec) bool {
	msg := g.toDriver.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DriverToDeviceMsg:
		g.processDriverMsg(msg, now)
	default:
		panic("Unhandled message type")
	}

	return true
}

func (g *GPU) processSMsInput(now sim.VTimeInSec) bool {
	msg := g.toSMs.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToDeviceMsg:
		g.processSMsMsg(msg, now)
		return true
	default:
		panic("Unhandled message type")
	}
}

func (g *GPU) processDriverMsg(msg *message.DriverToDeviceMsg, now sim.VTimeInSec) {
	for _, threadblock := range msg.Kernel.Threadblocks {
		g.undispatchedThreadblocks = append(g.undispatchedThreadblocks, &threadblock)
		g.unfinishedThreadblocksCount++
	}

	g.toDriver.Retrieve(now)
}

func (g *GPU) processSMsMsg(msg *message.SMToDeviceMsg, now sim.VTimeInSec) {
	if msg.ThreadblockFinished {
		g.freeSMs = append(g.freeSMs, g.sms[msg.SMID])
		g.unfinishedThreadblocksCount--
		if g.unfinishedThreadblocksCount == 0 {
			g.finishedKernelsCount++
		}
	}

	g.toSMs.Retrieve(now)
}

func (g *GPU) reportFinishedKernels(now sim.VTimeInSec) bool {
	if g.finishedKernelsCount == 0 {
		return false
	}

	msg := &message.DeviceToDriverMsg{
		KernelFinished: true,
		DeviceID:       g.ID,
	}
	msg.Src = g.toDriver
	msg.Dst = g.toDriverRemote
	msg.SendTime = now

	err := g.toDriver.Send(msg)
	if err != nil {
		return false
	}

	g.finishedKernelsCount--

	return true
}

func (g *GPU) dispatchThreadblocksToSMs(now sim.VTimeInSec) bool {
	if len(g.freeSMs) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}

	sm := g.freeSMs[0]
	threadblock := g.undispatchedThreadblocks[0]

	msg := &message.DeviceToSMMsg{
		Threadblock: *threadblock,
	}
	msg.Src = g.toSMs
	msg.Dst = sm.GetPortByName("ToGPU")
	msg.SendTime = now

	err := g.toSMs.Send(msg)
	if err != nil {
		return false
	}

	g.freeSMs = g.freeSMs[1:]
	g.undispatchedThreadblocks = g.undispatchedThreadblocks[1:]

	return false
}
