package gpu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/timing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"

	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/sm"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const warpSize = 32

// GPUController models the compute work distributor of a GPU. It receives
// kernels from the driver, dispatches their thread blocks to SMs, and reports
// finished kernels back to the driver.
type GPUController struct {
	*Comp
	SMList []*sm.SMController
}

type Spec struct {
	ToDriver                           messaging.RemotePort `json:"to_driver"`
	SMs                                []SMPeer             `json:"sms"`
	Freq                               timing.Freq          `json:"freq"`
	SMThreadCapacity                   uint64               `json:"sm_thread_capacity"`
	GPU2SMThreadBlockAllocationLatency uint64               `json:"gpu2sm_thread_block_allocation_latency"`
	GPUReceiveCTALatencyUnit           float64              `json:"gpu_receive_cta_latency_unit"`
	GPUReceiveSMLatency                uint64               `json:"gpu_receive_sm_latency"`
	CWDIssueWidth                      uint64               `json:"cwd_issue_width"`
	SMResponseHandleWidth              uint64               `json:"sm_response_handle_width"`
	MaxCTAPerSM                        uint64               `json:"max_cta_per_sm"`
}

// SMPeer is an SM identity and its dispatch port.
type SMPeer struct {
	Name string               `json:"name"`
	Port messaging.RemotePort `json:"port"`
}

type State struct {
	SMAssignedThreadTable                       map[string]uint64
	SMAssignedCTACountTable                     map[string]uint64
	SMIssueIndex                                int
	UndispatchedThreadblocks                    []trace.BlockRef
	UnfinishedThreadblocksCount                 uint64
	FinishedKernelsCount                        uint64
	GPU2SMThreadBlockAllocationLatencyRemaining uint64
	GPUReceiveCTALatencyRemaining               uint64
	GPUReceiveSMLatencyRemaining                uint64
}

type Resources struct{ Traces *trace.Store }
type Ports struct {
	ToDriver messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/nvidia/message.device"`
	ToSMs    messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/nvidia/message.device"`
}
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Middlewares struct{ Controller *GPUController }

func (m *Middlewares) Initialize(c *Comp) {
	m.Controller = &GPUController{Comp: c}
}
func (c *GPUController) Handle(timing.Event) bool { return c.Tick() }

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
	msg, ok := g.Ports.ToDriver.PeekIncoming()
	if !ok {
		return false
	}

	switch msg := msg.Payload.(type) {
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
func (g *GPUController) processKernel(id uint64) {
	kernel := g.Resources.Traces.Kernel(id)
	switch {
	case g.State.GPUReceiveCTALatencyRemaining == 1:
		g.State.GPUReceiveCTALatencyRemaining = 0
	case g.State.GPUReceiveCTALatencyRemaining > 0:
		g.State.GPUReceiveCTALatencyRemaining--
		return
	default:
		latency := uint64(g.Spec.GPUReceiveCTALatencyUnit *
			float64(len(kernel.Threadblocks)))
		if latency > 1 {
			g.State.GPUReceiveCTALatencyRemaining = latency - 1
			return
		}
	}

	for i := range kernel.Threadblocks {
		g.State.UndispatchedThreadblocks = append(g.State.UndispatchedThreadblocks, trace.BlockRef{Kernel: id, Block: i})
	}
	g.State.UnfinishedThreadblocksCount += kernel.ThreadblocksCount()

	if len(kernel.Threadblocks) == 0 {
		g.State.FinishedKernelsCount++
	}

	g.Ports.ToDriver.RetrieveIncoming()
}

func (g *GPUController) processSMsInput() bool {
	if _, ok := g.Ports.ToSMs.PeekIncoming(); !ok {
		return false
	}

	if g.State.GPUReceiveSMLatencyRemaining > 0 {
		g.State.GPUReceiveSMLatencyRemaining--
		return true
	}

	g.State.GPUReceiveSMLatencyRemaining = g.Spec.GPUReceiveSMLatency

	for range max(g.Spec.SMResponseHandleWidth, 1) {
		msg, ok := g.Ports.ToSMs.PeekIncoming()
		if !ok {
			break
		}

		switch msg := msg.Payload.(type) {
		case message.SMToDeviceMsg:
			g.processThreadblockFinished(msg)
		default:
			panic(fmt.Sprintf("%s: unexpected message %T from SM",
				g.Name(), msg))
		}

		g.Ports.ToSMs.RetrieveIncoming()
	}

	return true
}

func (g *GPUController) processThreadblockFinished(msg message.SMToDeviceMsg) {
	if msg.NumThreadFinished == 0 {
		return
	}

	if g.State.SMAssignedCTACountTable[msg.SMID] == 0 ||
		g.State.SMAssignedThreadTable[msg.SMID] < msg.NumThreadFinished {
		panic(fmt.Sprintf("%s: SM %s finished more work than assigned",
			g.Name(), msg.SMID))
	}

	g.State.SMAssignedThreadTable[msg.SMID] -= msg.NumThreadFinished
	g.State.SMAssignedCTACountTable[msg.SMID]--

	g.State.UnfinishedThreadblocksCount--
	if g.State.UnfinishedThreadblocksCount == 0 {
		g.State.FinishedKernelsCount++
	}
}

func (g *GPUController) reportFinishedKernels() bool {
	if g.State.FinishedKernelsCount == 0 || !g.Ports.ToDriver.CanSend() {
		return false
	}

	msg := messaging.Msg{
		ID:  g.NewID(),
		Src: g.Ports.ToDriver.AsRemote(),
		Dst: g.Spec.ToDriver,
		Payload: message.DeviceToDriverMsg{
			KernelFinished: true,
			DeviceID:       g.Name(),
		},
	}
	g.Ports.ToDriver.Send(msg)
	g.State.FinishedKernelsCount--

	return true
}

// pickSM finds the SM with the fewest resident thread blocks (then the fewest
// threads) that can still host a thread block of nThreads threads. It returns
// -1 if no SM has room.
func (g *GPUController) pickSM(nThreads uint64) int {
	bestIndex := -1
	bestCTAs := ^uint64(0)
	bestThreads := ^uint64(0)

	for i := range g.Spec.SMs {
		index := (g.State.SMIssueIndex + i) % len(g.Spec.SMs)
		smID := g.Spec.SMs[index].Name
		threads := g.State.SMAssignedThreadTable[smID]
		ctas := g.State.SMAssignedCTACountTable[smID]

		if threads+nThreads > g.Spec.SMThreadCapacity || ctas >= g.Spec.MaxCTAPerSM {
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
	if len(g.Spec.SMs) == 0 || len(g.State.UndispatchedThreadblocks) == 0 {
		return false
	}

	if g.State.GPU2SMThreadBlockAllocationLatencyRemaining > 0 {
		g.State.GPU2SMThreadBlockAllocationLatencyRemaining--
		return true
	}

	dispatched := uint64(0)

	for dispatched < max(g.Spec.CWDIssueWidth, 1) {
		if len(g.State.UndispatchedThreadblocks) == 0 || !g.Ports.ToSMs.CanSend() {
			break
		}

		tb := g.State.UndispatchedThreadblocks[0]
		nThreads := g.Resources.Traces.Block(tb).WarpsCount() * warpSize

		smIndex := g.pickSM(nThreads)
		if smIndex < 0 {
			break
		}

		target := g.Spec.SMs[smIndex]
		g.State.SMAssignedThreadTable[target.Name] += nThreads
		g.State.SMAssignedCTACountTable[target.Name]++
		g.State.SMIssueIndex = smIndex

		dst := target.Port
		g.Ports.ToSMs.Send(
			messaging.Msg{ID: g.NewID(), Src: g.Ports.ToSMs.AsRemote(), Dst: dst, Payload: message.DeviceToSMMsg{
				Threadblock: tb,
			}},
		)

		g.State.UndispatchedThreadblocks = g.State.UndispatchedThreadblocks[1:]
		dispatched++
	}

	// When no SM has room, wait for an SM to report a finished thread block.
	if dispatched == 0 {
		return false
	}

	g.State.GPU2SMThreadBlockAllocationLatencyRemaining =
		g.Spec.GPU2SMThreadBlockAllocationLatency

	return true
}

// Status reports the thread blocks that wait for an SM and the thread blocks
// that have not finished yet.
func (g *GPUController) Status() (undispatched, unfinished uint64) {
	return uint64(len(g.State.UndispatchedThreadblocks)), g.State.UnfinishedThreadblocksCount
}

func (s *State) Initialize(c *Comp) {
	if c.Resources.Traces == nil {
		panic("NVIDIA component requires a shared trace store")
	}
	if c.Spec.ToDriver == "" {
		panic(fmt.Sprintf("GPU requires a driver destination: to_driver=%q", c.Spec.ToDriver))
	}

	names := make(map[string]bool)
	ports := make(map[messaging.RemotePort]bool)
	for _, peer := range c.Spec.SMs {
		if peer.Name == "" || peer.Port == "" || names[peer.Name] || ports[peer.Port] {
			panic(fmt.Sprintf("invalid or duplicate GPU peer: name=%q port=%q", peer.Name, peer.Port))
		}
		names[peer.Name] = true
		ports[peer.Port] = true
	}

	s.SMAssignedThreadTable = make(map[string]uint64)
	s.SMAssignedCTACountTable = make(map[string]uint64)
	s.GPU2SMThreadBlockAllocationLatencyRemaining = c.Spec.GPU2SMThreadBlockAllocationLatency
	s.GPUReceiveSMLatencyRemaining = c.Spec.GPUReceiveSMLatency
}
