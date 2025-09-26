package gpu

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type GPUController struct {
	*sim.TickingComponent

	ID      string
	gpuName string

	// meta
	toDriver       sim.Port
	toDriverRemote sim.Port

	toSMs sim.Port
	SMs   map[string]*sm.SMController
	// freeSMs []*sm.SMController
	SMList                  []*sm.SMController
	SMAssignedThreadTable   map[string]uint64
	SMAssignedCTACountTable map[string]uint64

	ToSMSPs sim.Port

	// cache updates
	// ToCaches sim.Port // used to send cache reqs
	ToDRAM sim.Port // remote DRAM's port

	// ToSMSPsMem sim.Port // used to receive and send mem reqs to SMSPs

	// toSMMem       sim.Port
	// toSMMemRemote sim.Port

	// toDRAM       sim.Port
	// toDRAMRemote sim.Port

	// L2Caches    []*writeback.Comp
	// L2CacheSize uint64
	// Drams    []*dram.Comp
	// DramSize uint64

	// PendingReadReq  map[string]*mem.ReadReq
	// PendingWriteReq map[string]*mem.WriteReq

	// PendingSMSPtoGPUControllerMemReadReq  map[string]*message.SMSPToGPUControllerMemReadMsg
	// PendingSMSPtoGPUControllerMemWriteReq map[string]*message.SMSPToGPUControllerMemWriteMsg

	// PendingCacheReadReq  map[string]*message.GPUControllerToCachesMemReadMsg
	// PendingCacheWriteReq map[string]*message.GPUControllerToCachesMemWriteMsg

	// RDMAEngine *rdma.Comp

	undispatchedThreadblocks    []*trace.ThreadblockTrace
	unfinishedThreadblocksCount uint64

	finishedKernelsCount uint64

	SMIssueIndex uint64
	smsCount     uint64

	// launchOverheadLatency          uint64
	// launchOverheadLatencyRemaining uint64

	SMThreadCapacity                            uint64
	GPU2SMThreadBlockAllocationLatency          uint64
	GPU2SMThreadBlockAllocationLatencyRemaining uint64

	GPUReceiveSMLatency          uint64
	GPUReceiveSMLatencyRemaining uint64
}

func (g *GPUController) SetDriverRemotePort(remote sim.Port) {
	g.toDriverRemote = remote
}

func (g *GPUController) Tick() bool {
	madeProgress := false
	madeProgress = g.reportFinishedKernels() || madeProgress
	madeProgress = g.dispatchThreadblocksToSMs() || madeProgress
	madeProgress = g.processDriverInput() || madeProgress
	madeProgress = g.processSMsInput() || madeProgress
	madeProgress = g.checkAnySMAssigned() || madeProgress
	// madeProgress = g.processCaches() || madeProgress
	// madeProgress = g.processSMsInputMem() || madeProgress
	// madeProgress = g.processDRAMRsp() || madeProgress

	return madeProgress
}

func (g *GPUController) checkAnySMAssigned() bool {
	for _, assigned := range g.SMAssignedThreadTable {
		if assigned > 0 {
			return true
		}
	}
	return false
}

func (g *GPUController) processDriverInput() bool {
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

func (g *GPUController) processSMsInput() bool {
	msg := g.toSMs.PeekIncoming()
	if msg == nil {
		return false
	}

	if g.GPUReceiveSMLatencyRemaining > 0 {
		g.GPUReceiveSMLatencyRemaining--
		return true
	}

	switch msg := msg.(type) {
	case *message.SMToDeviceMsg:
		g.processSMsMsg(msg)
	default:
		log.WithField("function", "processSMsInput").Panic("Unhandled message type")
	}

	g.GPUReceiveSMLatencyRemaining = g.GPUReceiveSMLatency

	return true
}

func (g *GPUController) processDriverMsg(msg *message.DriverToDeviceMsg) {
	for i := range msg.Kernel.Threadblocks {
		g.undispatchedThreadblocks = append(g.undispatchedThreadblocks, msg.Kernel.Threadblocks[i])
		g.unfinishedThreadblocksCount++
		// fmt.Printf("%.10f, %s, GPUController, received a msg from driver for a kernel with %d threadblocks, unfinished threadblocks count = %d->%d\n", g.Engine.CurrentTime(), g.Name(), len(msg.Kernel.Threadblocks), g.unfinishedThreadblocksCount-1, g.unfinishedThreadblocksCount)
	}
	g.toDriver.RetrieveIncoming()
}

func (g *GPUController) processSMsMsg(msg *message.SMToDeviceMsg) {
	if msg.NumThreadFinished > 0 {
		// g.freeSMs = append(g.freeSMs, g.SMs[msg.SMID])
		g.unfinishedThreadblocksCount--
		// fmt.Printf("%.10f, %s, GPUController, received a msg from sm for a threadblock finished, unfinished threadblocks count = %d->%d\n", g.Engine.CurrentTime(), g.Name(), g.unfinishedThreadblocksCount+1, g.unfinishedThreadblocksCount)
		if g.unfinishedThreadblocksCount == 0 {
			g.finishedKernelsCount++
		}
		// fmt.Printf("g.SMAssignedThreadTable[%s] = %d, msg.NumThreadFinished = %d, g.SMAssignedThreadTable[%s]-=msg.NumThreadFinished=%d\n", msg.SMID, g.SMAssignedThreadTable[msg.SMID], msg.NumThreadFinished, msg.SMID, g.SMAssignedThreadTable[msg.SMID]-msg.NumThreadFinished)
		g.SMAssignedThreadTable[msg.SMID] -= msg.NumThreadFinished
		g.SMAssignedCTACountTable[msg.SMID] -= 1
		if g.SMAssignedThreadTable[msg.SMID] < 0 || g.SMAssignedCTACountTable[msg.SMID] < 0 {
			log.Panic(fmt.Sprintf("SMAssignedThreadTable[%s] < 0 || SMAssignedCTACountTable[%s] < 0", msg.SMID, msg.SMID))
		}
	}
	g.toSMs.RetrieveIncoming()
}

// func (g *GPUController) processDRAMWriteRspMsg(rspMsg *mem.WriteDoneRsp) bool {
// 	fmt.Printf("%.10f, %s, GPUController\n", g.Engine.CurrentTime(), g.Name())
// 	msg := &message.GPUtoSMMemWriteMsg{
// 		Address: g.PendingReadReq[rspMsg.RespondTo].Address,
// 		Rsp:     *rspMsg,
// 	}
// 	msg.Src = g.toSMMem.AsRemote()
// 	msg.Dst = g.PendingReadReq[rspMsg.RespondTo].Src
// 	// err := g.toSMMem.Send(msg)
// 	// if err != nil {
// 	// 	fmt.Printf("GPUController failed to send write rsp to SMController: %v\n", err)
// 	// 	g.toDRAM.RetrieveIncoming()
// 	// 	return false
// 	// }
// 	g.toDRAM.RetrieveIncoming()
// 	return true
// }

func (g *GPUController) reportFinishedKernels() bool {
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

func (g *GPUController) issueSMIndex(nThreadToBeAssigned uint64) int {
	for i := 0; i < int(g.smsCount); i++ {
		index := (int(g.SMIssueIndex) + i) % int(g.smsCount)
		sm := g.SMList[index]
		if g.SMAssignedThreadTable[sm.ID]+nThreadToBeAssigned <= g.SMThreadCapacity && g.SMAssignedCTACountTable[sm.ID] < 4 {
			g.SMAssignedThreadTable[sm.ID] += nThreadToBeAssigned
			g.SMAssignedCTACountTable[sm.ID] += 1
			// fmt.Printf("g.SMAssignedThreadTable[%s] = %d, nThreadToBeAssigned = %d, g.SMAssignedThreadTable[%s]+=nThreadToBeAssigned=%d\n", sm.ID, g.SMAssignedThreadTable[sm.ID], nThreadToBeAssigned, sm.ID, g.SMAssignedThreadTable[sm.ID]+nThreadToBeAssigned)
			g.SMIssueIndex = uint64((index + 1) % int(g.smsCount))
			return index
		}
		// fmt.Printf("sm %s cannot take %d threads now, current assigned threads = %d, current assigned CTAs = %d\n", sm.ID, nThreadToBeAssigned, g.SMAssignedThreadTable[sm.ID], g.SMAssignedCTACountTable[sm.ID])
	}
	// fmt.Printf("All sms already has full threadblocks to do\n")
	return -1
}

func (g *GPUController) dispatchThreadblocksToSMs() bool {
	if len(g.SMList) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}
	if g.smsCount == 0 {
		log.Panic("SM count is 0")
	}
	if g.GPU2SMThreadBlockAllocationLatencyRemaining > 0 {
		g.GPU2SMThreadBlockAllocationLatencyRemaining--
		return true
	}

	threadblock := g.undispatchedThreadblocks[0]
	nThreadToBeAssigned := threadblock.WarpsCount() * 32

	smIndex := g.issueSMIndex(nThreadToBeAssigned)
	if smIndex == -1 {
		// All sms already has a threadblock to do
		return true
	}
	sm := g.SMList[smIndex]

	msg := &message.DeviceToSMMsg{
		Threadblock: *threadblock,
	}
	msg.Src = g.toSMs.AsRemote()
	msg.Dst = sm.GetPortByName(fmt.Sprintf("%s.ToGPU", sm.Name())).AsRemote()

	err := g.toSMs.Send(msg)
	if err != nil {
		return false
	}

	// g.freeSMs = g.freeSMs[1:]
	g.SMIssueIndex = (g.SMIssueIndex + 1) % g.smsCount

	g.undispatchedThreadblocks = g.undispatchedThreadblocks[1:]

	g.GPU2SMThreadBlockAllocationLatencyRemaining = g.GPU2SMThreadBlockAllocationLatency

	return false
}

func (g *GPUController) LogStatus() {
}
