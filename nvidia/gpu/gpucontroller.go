package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/sm"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const warpSize = 32

// GPUController models the compute work distributor of a GPU. It receives
// kernels from the driver, dispatches their thread blocks to SMs, and reports
// finished kernels back to the driver.
type GPUController struct {
	*modeling.TickingComponent

	ID string

	toDriver       messaging.Port
	toDriverRemote messaging.RemotePort
	toSMs          messaging.Port

	SMList                  []*sm.SMController
	SMAssignedThreadTable   map[string]uint64
	SMAssignedCTACountTable map[string]uint64
	SMIssueIndex            int
	SMThreadCapacity        uint64

	undispatchedThreadblocks    []*trace.ThreadblockTrace
	unfinishedThreadblocksCount uint64
	finishedKernelsCount        uint64

	GPU2SMThreadBlockAllocationLatency          uint64
	GPU2SMThreadBlockAllocationLatencyRemaining uint64

	GPUReceiveCTALatencyUnit      float64
	GPUReceiveCTALatencyRemaining uint64

	GPUReceiveSMLatency          uint64
	GPUReceiveSMLatencyRemaining uint64

	CWDIssueWidth         uint64
	SMResponseHandleWidth uint64
	MaxCTAPerSM           uint64
}

// SetDriverRemotePort sets the driver port that finished kernels are
// reported to.
func (g *GPUController) SetDriverRemotePort(remote messaging.RemotePort) {
	g.toDriverRemote = remote
}

// Tick advances the GPU controller by one cycle.
func (g *GPUController) Tick() bool {
	madeProgress := false
	madeProgress = g.reportFinishedKernels() || madeProgress
	madeProgress = g.dispatchThreadblocksToSMs() || madeProgress
	madeProgress = g.processDriverInput() || madeProgress
	madeProgress = g.processSMsInput() || madeProgress

	return madeProgress
}

func (g *GPUController) processDriverInput() bool {
	msg := g.toDriver.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case message.DriverToDeviceMsg:
		g.processKernel(msg.Kernel)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from driver",
			g.Name(), msg))
	}

	return true
}

// processKernel accepts a kernel after a delay proportional to its number of
// thread blocks.
func (g *GPUController) processKernel(kernel *trace.KernelTrace) {
	switch {
	case g.GPUReceiveCTALatencyRemaining == 1:
		g.GPUReceiveCTALatencyRemaining = 0
	case g.GPUReceiveCTALatencyRemaining > 0:
		g.GPUReceiveCTALatencyRemaining--
		return
	default:
		latency := uint64(g.GPUReceiveCTALatencyUnit *
			float64(len(kernel.Threadblocks)))
		if latency > 1 {
			g.GPUReceiveCTALatencyRemaining = latency - 1
			return
		}
	}

	g.undispatchedThreadblocks = append(
		g.undispatchedThreadblocks, kernel.Threadblocks...)
	g.unfinishedThreadblocksCount += kernel.ThreadblocksCount()

	if len(kernel.Threadblocks) == 0 {
		g.finishedKernelsCount++
	}

	g.toDriver.RetrieveIncoming()
}

func (g *GPUController) processSMsInput() bool {
	if g.toSMs.PeekIncoming() == nil {
		return false
	}

	if g.GPUReceiveSMLatencyRemaining > 0 {
		g.GPUReceiveSMLatencyRemaining--
		return true
	}

	g.GPUReceiveSMLatencyRemaining = g.GPUReceiveSMLatency

	for range max(g.SMResponseHandleWidth, 1) {
		msg := g.toSMs.PeekIncoming()
		if msg == nil {
			break
		}

		switch msg := msg.(type) {
		case message.SMToDeviceMsg:
			g.processThreadblockFinished(msg)
		default:
			panic(fmt.Sprintf("%s: unexpected message %T from SM",
				g.Name(), msg))
		}

		g.toSMs.RetrieveIncoming()
	}

	return true
}

func (g *GPUController) processThreadblockFinished(msg message.SMToDeviceMsg) {
	if msg.NumThreadFinished == 0 {
		return
	}

	if g.SMAssignedCTACountTable[msg.SMID] == 0 ||
		g.SMAssignedThreadTable[msg.SMID] < msg.NumThreadFinished {
		panic(fmt.Sprintf("%s: SM %s finished more work than assigned",
			g.Name(), msg.SMID))
	}

	g.SMAssignedThreadTable[msg.SMID] -= msg.NumThreadFinished
	g.SMAssignedCTACountTable[msg.SMID]--

	g.unfinishedThreadblocksCount--
	if g.unfinishedThreadblocksCount == 0 {
		g.finishedKernelsCount++
	}
}

func (g *GPUController) reportFinishedKernels() bool {
	if g.finishedKernelsCount == 0 || !g.toDriver.CanSend() {
		return false
	}

	msg := message.DeviceToDriverMsg{
		MsgMeta:        message.NewMeta(g.toDriver.AsRemote(), g.toDriverRemote),
		KernelFinished: true,
		DeviceID:       g.ID,
	}
	g.toDriver.Send(msg)
	g.finishedKernelsCount--

	return true
}

// pickSM finds the SM with the fewest resident thread blocks (then the fewest
// threads) that can still host a thread block of nThreads threads. It returns
// -1 if no SM has room.
func (g *GPUController) pickSM(nThreads uint64) int {
	bestIndex := -1
	bestCTAs := ^uint64(0)
	bestThreads := ^uint64(0)

	for i := range g.SMList {
		index := (g.SMIssueIndex + i) % len(g.SMList)
		smID := g.SMList[index].ID
		threads := g.SMAssignedThreadTable[smID]
		ctas := g.SMAssignedCTACountTable[smID]

		if threads+nThreads > g.SMThreadCapacity || ctas >= g.MaxCTAPerSM {
			continue
		}

		if ctas < bestCTAs || (ctas == bestCTAs && threads < bestThreads) {
			bestIndex = index
			bestCTAs = ctas
			bestThreads = threads
		}
	}

	return bestIndex
}

func (g *GPUController) dispatchThreadblocksToSMs() bool {
	if len(g.SMList) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}

	if g.GPU2SMThreadBlockAllocationLatencyRemaining > 0 {
		g.GPU2SMThreadBlockAllocationLatencyRemaining--
		return true
	}

	dispatched := uint64(0)

	for dispatched < max(g.CWDIssueWidth, 1) {
		if len(g.undispatchedThreadblocks) == 0 || !g.toSMs.CanSend() {
			break
		}

		tb := g.undispatchedThreadblocks[0]
		nThreads := tb.WarpsCount() * warpSize

		smIndex := g.pickSM(nThreads)
		if smIndex < 0 {
			break
		}

		target := g.SMList[smIndex]
		g.SMAssignedThreadTable[target.ID] += nThreads
		g.SMAssignedCTACountTable[target.ID]++
		g.SMIssueIndex = smIndex

		dst := target.GetPortByName("ToGPU").AsRemote()
		g.toSMs.Send(message.DeviceToSMMsg{
			MsgMeta:     message.NewMeta(g.toSMs.AsRemote(), dst),
			Threadblock: tb,
		})

		g.undispatchedThreadblocks = g.undispatchedThreadblocks[1:]
		dispatched++
	}

	// When no SM has room, wait for an SM to report a finished thread block.
	if dispatched == 0 {
		return false
	}

	g.GPU2SMThreadBlockAllocationLatencyRemaining =
		g.GPU2SMThreadBlockAllocationLatency

	return true
}
