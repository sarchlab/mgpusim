package gpu

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/mem/dram"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rdma"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"

	"github.com/sarchlab/akita/v4/mem/cache/writeback"
)

type GPU struct {
	*sim.TickingComponent

	ID      string
	gpuName string

	// meta
	toDriver       sim.Port
	toDriverRemote sim.Port

	toSMs   sim.Port
	SMs     map[string]*sm.SM
	freeSMs []*sm.SM

	L2Caches    []*writeback.Comp
	L2CacheSize uint64
	Drams       []*dram.Comp
	DramSize    uint64

	RDMAEngine *rdma.Comp

	undispatchedThreadblocks    []*trace.ThreadblockTrace
	unfinishedThreadblocksCount uint64

	finishedKernelsCount uint64
}

func (g *GPU) SetDriverRemotePort(remote sim.Port) {
	g.toDriverRemote = remote
}

func (g *GPU) Tick() bool {
	madeProgress := false
	madeProgress = g.reportFinishedKernels() || madeProgress
	madeProgress = g.dispatchThreadblocksToSMs() || madeProgress
	madeProgress = g.processDriverInput() || madeProgress
	madeProgress = g.processSMsInput() || madeProgress

	return madeProgress
}

func (g *GPU) processDriverInput() bool {
	msg := g.toDriver.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DriverToDeviceMsg:
		g.processDriverMsg(msg)
	default:
		log.WithField("function", "processDriverInput").Panic("Unhandled message type")
	}

	return true
}

func (g *GPU) processSMsInput() bool {
	msg := g.toSMs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToDeviceMsg:
		g.processSMsMsg(msg)
	default:
		log.WithField("function", "processSMsInput").Panic("Unhandled message type")
	}

	return true
}

func (g *GPU) processDriverMsg(msg *message.DriverToDeviceMsg) {
	for i := range msg.Kernel.Threadblocks {
		g.undispatchedThreadblocks = append(g.undispatchedThreadblocks, msg.Kernel.Threadblocks[i])
		g.unfinishedThreadblocksCount++
	}
	g.toDriver.RetrieveIncoming()
}

func (g *GPU) processSMsMsg(msg *message.SMToDeviceMsg) {
	if msg.ThreadblockFinished {
		g.freeSMs = append(g.freeSMs, g.SMs[msg.SMID])
		g.unfinishedThreadblocksCount--
		if g.unfinishedThreadblocksCount == 0 {
			g.finishedKernelsCount++
		}
	}
	g.toSMs.RetrieveIncoming()
}

func (g *GPU) reportFinishedKernels() bool {
	if g.finishedKernelsCount == 0 {
		return false
	}

	msg := &message.DeviceToDriverMsg{
		KernelFinished: true,
		DeviceID:       g.ID,
	}
	msg.Src = g.toDriver.AsRemote()
	msg.Dst = g.toDriverRemote.AsRemote()

	err := g.toDriver.Send(msg)
	if err != nil {
		return false
	}

	g.finishedKernelsCount--

	return true
}

func (g *GPU) dispatchThreadblocksToSMs() bool {
	if len(g.freeSMs) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}

	sm := g.freeSMs[0]
	threadblock := g.undispatchedThreadblocks[0]

	msg := &message.DeviceToSMMsg{
		Threadblock: *threadblock,
	}
	msg.Src = g.toSMs.AsRemote()
	msg.Dst = sm.GetPortByName(fmt.Sprintf("%s.ToGPU", sm.Name())).AsRemote()

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
