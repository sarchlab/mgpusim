package gpu

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/message"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/sm"
)

type GPU struct {
	*sim.TickingComponent

	ID string

	// meta
	toDriver       sim.Port
	toDriverRemote sim.Port

	toSMs   sim.Port
	SMs     map[string]*sm.SM
	freeSMs []*sm.SM

	undispatchedThreadblocks    []*nvidia.Threadblock
	unfinishedThreadblocksCount int64

	finishedKernelsCount int64
}

func (g *GPU) SetDriverRemotePort(remote sim.Port) {
	g.toDriverRemote = remote
}

// v3
// func (g *GPU) Tick(now sim.VTimeInSec) bool {
func (g *GPU) Tick() bool {
	madeProgress := false
    // v3
    //  madeProgress = g.reportFinishedKernels(now) || madeProgress
    // 	madeProgress = g.dispatchThreadblocksToSMs(now) || madeProgress
    // 	madeProgress = g.processDriverInput(now) || madeProgress
    // 	madeProgress = g.processSMsInput(now) || madeProgress
	madeProgress = g.reportFinishedKernels() || madeProgress
	madeProgress = g.dispatchThreadblocksToSMs() || madeProgress
	madeProgress = g.processDriverInput() || madeProgress
	madeProgress = g.processSMsInput() || madeProgress

	return madeProgress
}

// v3
// func (g *GPU) processDriverInput(now sim.VTimeInSec) bool {
func (g *GPU) processDriverInput() bool {
    // v3
    // msg := g.toDriver.Peek()
	msg := g.toDriver.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DriverToDeviceMsg:
		g.processDriverMsg(msg)
		// v3
		// g.processDriverMsg(msg, now)
	default:
		log.WithField("function", "processDriverInput").Panic("Unhandled message type")
	}

	return true
}

// v3
// func (g *GPU) processSMsInput(now sim.VTimeInSec) bool {
func (g *GPU) processSMsInput() bool {
    // v3
    // msg := g.toSMs.Peek()
	msg := g.toSMs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToDeviceMsg:
		g.processSMsMsg(msg)
		// v3
		// g.processSMsMsg(msg, now)
	default:
		log.WithField("function", "processSMsInput").Panic("Unhandled message type")
	}

	return true
}

// v3
// func (g *GPU) processDriverMsg(msg *message.DriverToDeviceMsg, now sim.VTimeInSec) {
func (g *GPU) processDriverMsg(msg *message.DriverToDeviceMsg) {
	for i := range msg.Kernel.Threadblocks {
		g.undispatchedThreadblocks = append(g.undispatchedThreadblocks, &msg.Kernel.Threadblocks[i])
		g.unfinishedThreadblocksCount++
	}
    // v3
    // g.toDriver.Retrieve(now)
	g.toDriver.RetrieveIncoming()
}

// v3
// func (g *GPU) processSMsMsg(msg *message.SMToDeviceMsg, now sim.VTimeInSec) {
func (g *GPU) processSMsMsg(msg *message.SMToDeviceMsg) {
	if msg.ThreadblockFinished {
		g.freeSMs = append(g.freeSMs, g.SMs[msg.SMID])
		g.unfinishedThreadblocksCount--
		if g.unfinishedThreadblocksCount == 0 {
			g.finishedKernelsCount++
		}
	}
    // v3
    // g.toSMs.Retrieve(now)
	g.toSMs.RetrieveIncoming()
}

// v3
// func (g *GPU) reportFinishedKernels(now sim.VTimeInSec) bool {
func (g *GPU) reportFinishedKernels() bool {
	if g.finishedKernelsCount == 0 {
		return false
	}

	msg := &message.DeviceToDriverMsg{
		KernelFinished: true,
		DeviceID:       g.ID,
	}
	// v3
	// msg.Src = g.toDriver
	// msg.Dst = g.toDriverRemote
	msg.Src = g.toDriver.AsRemote()
	msg.Dst = g.toDriverRemote.AsRemote()
	// v3
    // 	msg.SendTime = now

	err := g.toDriver.Send(msg)
	if err != nil {
		return false
	}

	g.finishedKernelsCount--

	return true
}

// v3
// func (g *GPU) dispatchThreadblocksToSMs(now sim.VTimeInSec) bool {
func (g *GPU) dispatchThreadblocksToSMs() bool {
	if len(g.freeSMs) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}

	sm := g.freeSMs[0]
	threadblock := g.undispatchedThreadblocks[0]

	msg := &message.DeviceToSMMsg{
		Threadblock: *threadblock,
	}
	// v3
	// msg.Src = g.toSMs
	// msg.Dst = sm.GetPortByName("ToGPU")
	msg.Src = g.toSMs.AsRemote()
	msg.Dst = sm.GetPortByName("ToGPU").AsRemote()
	// v3
    // 	msg.SendTime = now

	err := g.toSMs.Send(msg)
	if err != nil {
		return false
	}

	g.freeSMs = g.freeSMs[1:]
	g.undispatchedThreadblocks = g.undispatchedThreadblocks[1:]

	return false
}

func (g *GPU) LogStatus() {
}
