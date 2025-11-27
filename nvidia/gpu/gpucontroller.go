package gpu

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/sm"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

const DispatchThreadblocksToSMsStrategy = "BestOne" // "BestOne" or "Average"

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

	GPUReceiveCTALatencyUnit      float64
	GPUReceiveCTALatencyRemaining uint64

	GPUReceiveSMLatency          uint64
	GPUReceiveSMLatencyRemaining uint64

	CWDIssueWidth         uint64
	SMResponseHandleWidth uint64
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
	if g.GPUReceiveSMLatencyRemaining > 0 {
		g.GPUReceiveSMLatencyRemaining--
		return true
	}

	msg := g.toSMs.PeekIncoming()
	if msg == nil {
		return false
	}

	g.GPUReceiveSMLatencyRemaining = g.GPUReceiveSMLatency

	for i := 0; i < int(g.SMResponseHandleWidth); i++ {
		// fmt.Printf("i=%d\n", i)
		msg := g.toSMs.PeekIncoming()
		if msg == nil {
			return true
		}

		switch msg := msg.(type) {
		case *message.SMToDeviceMsg:
			g.processSMsMsg(msg)
		default:
			log.WithField("function", "processSMsInput").Panic("Unhandled message type")
		}

	}

	return true
}

func (g *GPUController) processDriverMsg(msg *message.DriverToDeviceMsg) {
	nKernelThreadblocks := len(msg.Kernel.Threadblocks)
	// fmt.Printf("g.GPUReceiveCTALatencyRemaining = %d, nKernelThreadblocks = %d\n", g.GPUReceiveCTALatencyRemaining, nKernelThreadblocks)
	if g.GPUReceiveCTALatencyRemaining == 1 {
		g.GPUReceiveCTALatencyRemaining = 0
	} else if g.GPUReceiveCTALatencyRemaining > 0 {
		g.GPUReceiveCTALatencyRemaining--
		return
	} else {

		g.GPUReceiveCTALatencyRemaining = uint64(g.GPUReceiveCTALatencyUnit * float64(nKernelThreadblocks)) // -1
		if g.GPUReceiveCTALatencyRemaining > 1 {
			g.GPUReceiveCTALatencyRemaining--
			return
		}
		g.GPUReceiveCTALatencyRemaining = 0
	}
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

func (g *GPUController) issueSMIndexBestOneStrategy(nThreadToBeAssigned uint64, nCTAToBeAssigned uint64) int {
	for i := 0; i < int(g.smsCount); i++ {
		index := (int(g.SMIssueIndex) + i) % int(g.smsCount)
		sm := g.SMList[index]
		if g.SMAssignedThreadTable[sm.ID]+nThreadToBeAssigned <= g.SMThreadCapacity && g.SMAssignedCTACountTable[sm.ID]+nCTAToBeAssigned <= 4 {
			g.SMAssignedThreadTable[sm.ID] += nThreadToBeAssigned
			g.SMAssignedCTACountTable[sm.ID] += nCTAToBeAssigned
			// fmt.Printf("g.SMAssignedThreadTable[%s] = %d, nThreadToBeAssigned = %d, g.SMAssignedThreadTable[%s]+=nThreadToBeAssigned=%d\n", sm.ID, g.SMAssignedThreadTable[sm.ID], nThreadToBeAssigned, sm.ID, g.SMAssignedThreadTable[sm.ID]+nThreadToBeAssigned)
			g.SMIssueIndex = uint64((index) % int(g.smsCount))
			return index
		}
		// fmt.Printf("sm %s cannot take %d threads now, current assigned threads = %d, current assigned CTAs = %d\n", sm.ID, nThreadToBeAssigned, g.SMAssignedThreadTable[sm.ID], g.SMAssignedCTACountTable[sm.ID])
	}
	// fmt.Printf("All sms already has full threadblocks to do\n")
	return -1
}

// func (g *GPUController) getAverageAssignedThread(SMAssignedThreadTable map[string]uint64) float64 {
// 	totalThreads := uint64(0)
// 	for _, assigned := range SMAssignedThreadTable {
// 		totalThreads += assigned
// 	}
// 	return float64(totalThreads) / float64(len(SMAssignedThreadTable))
// }

func (g *GPUController) issueSMIndexDictAverageStrategy(nCTACandidates int) (uint64, int, map[int]int) {
	// returns: nThreadToBeAssigned, nCTAToBeAssigned, map[threadIndex]SMIndex
	assignMap := make(map[int]int)
	nThreadToBeAssigned := uint64(0)
	nCTAToBeAssigned := 0

	if g.smsCount == 0 || nCTACandidates == 0 {
		return nThreadToBeAssigned, nCTAToBeAssigned, assignMap
	}

	// compute total threads in the candidate CTAs
	totalCandidateThreads := uint64(0)
	for i := 0; i < nCTACandidates && i < len(g.undispatchedThreadblocks); i++ {
		totalCandidateThreads += uint64(g.undispatchedThreadblocks[i].WarpsCount()) * 32
	}

	// current assigned threads
	nThreadCurrentlyAssigned := uint64(0)
	for i := 0; i < int(g.smsCount); i++ {
		sm := g.SMList[i]
		nThreadCurrentlyAssigned += g.SMAssignedThreadTable[sm.ID]
	}

	targetAvg := float64(nThreadCurrentlyAssigned+totalCandidateThreads) / float64(g.smsCount)
	if targetAvg > float64(g.SMThreadCapacity) {
		targetAvg = float64(g.SMThreadCapacity)
	}
	targetAvgUInt := uint64(targetAvg + 0.000001) // floor-ish but safe for comparison

	// Try to assign each CTA (in order) to a SM starting from rotating SMIssueIndex+i.
	// First pass: prefer SMs whose assigned threads < targetAvg and that have CTA slot and capacity.
	// Second pass: relax targetAvg requirement and assign to any SM with capacity & CTA slot.
	for pass := 1; pass <= 2; pass++ {
		for cIdx := 0; cIdx < nCTACandidates && cIdx < len(g.undispatchedThreadblocks); cIdx++ {
			// already assigned this CTA in previous pass?
			if cIdx < nCTAToBeAssigned {
				// already assigned (we keep CTAs contiguous from start), skip
				continue
			}

			cta := g.undispatchedThreadblocks[cIdx]
			ctaThreads := uint64(cta.WarpsCount()) * 32

			start := (int(g.SMIssueIndex) + cIdx) % int(g.smsCount)
			assigned := false
			for s := 0; s < int(g.smsCount); s++ {
				idx := (start + s) % int(g.smsCount)
				sm := g.SMList[idx]
				assignedThreads := g.SMAssignedThreadTable[sm.ID]
				assignedCTAs := g.SMAssignedCTACountTable[sm.ID]

				// capacity and CTA slot check
				if assignedThreads+ctaThreads > g.SMThreadCapacity || assignedCTAs >= 4 {
					continue
				}

				// pass 1: prefer below-target SMs
				if pass == 1 {
					if assignedThreads >= targetAvgUInt {
						continue
					}
				}

				// assign CTA to this SM
				g.SMAssignedThreadTable[sm.ID] += ctaThreads
				g.SMAssignedCTACountTable[sm.ID] += 1

				assignMap[nCTAToBeAssigned] = idx
				nThreadToBeAssigned += ctaThreads
				nCTAToBeAssigned++
				assigned = true
				break
			}
			if !assigned && pass == 2 {
				// cannot assign this CTA at all (all SMs full or CTA slots full) -> stop trying further CTAs
				break
			}
		}
		// if we already assigned some CTAs in first pass, second pass may fill more. Continue.
	}

	return nThreadToBeAssigned, nCTAToBeAssigned, assignMap
}

func (g *GPUController) dispatchThreadblocksToSMs() bool {
	if len(g.SMList) == 0 || len(g.undispatchedThreadblocks) == 0 {
		return false
	}
	if g.smsCount == 0 {
		log.Panic("SM count is 0")
	}
	// fmt.Printf("g.GPU2SMThreadBlockAllocationLatencyRemaining = %d\n", g.GPU2SMThreadBlockAllocationLatencyRemaining)
	if g.GPU2SMThreadBlockAllocationLatencyRemaining > 0 {
		g.GPU2SMThreadBlockAllocationLatencyRemaining--
		return true
	}

	// for i := uint64(0); i < g.CWDIssueWidth; i++ {
	// if len(g.undispatchedThreadblocks) == 0 {
	// 	break
	// }
	var threadblockList []*trace.ThreadblockTrace
	warpCount := uint64(0)
	if DispatchThreadblocksToSMsStrategy == "BestOne" {
		for i := uint64(0); i < g.CWDIssueWidth; i++ {
			if i >= uint64(len(g.undispatchedThreadblocks)) || (warpCount+g.undispatchedThreadblocks[i].WarpsCount())*32 > g.SMThreadCapacity {
				break
			}
			warpCount += g.undispatchedThreadblocks[i].WarpsCount()
			threadblockList = append(threadblockList, g.undispatchedThreadblocks[i])
		}

		// threadblock_0 := g.undispatchedThreadblocks[0]
		nThreadToBeAssigned := warpCount * 32

		smIndex := g.issueSMIndexBestOneStrategy(nThreadToBeAssigned, uint64(len(threadblockList)))
		// fmt.Printf("smIndex: %d\n", smIndex)
		if smIndex == -1 {
			// All sms already has a threadblock to do
			return true
		}

		for i := 0; i < len(threadblockList); i++ {
			threadblock := threadblockList[i]
			if len(g.undispatchedThreadblocks) == 0 {
				break
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
		}

		// g.freeSMs = g.freeSMs[1:]
		// fmt.Printf("Issued %d threadblocks\n", len(threadblockList))
		g.SMIssueIndex = (g.SMIssueIndex + 1) % g.smsCount

		g.undispatchedThreadblocks = g.undispatchedThreadblocks[len(threadblockList):]
	} else if DispatchThreadblocksToSMsStrategy == "Average" {
		// collect up to CWDIssueWidth CTAs as candidates
		maxCandidates := 40 // int(g.CWDIssueWidth)
		availableCTAs := len(g.undispatchedThreadblocks)
		if maxCandidates > availableCTAs {
			maxCandidates = availableCTAs
		}
		if maxCandidates == 0 {
			// nothing to do
		} else {
			_, nCTAToBeAssigned, assignMap := g.issueSMIndexDictAverageStrategy(maxCandidates)
			// fmt.Printf("%.10f, AverageThread: %.2f, nCTAToBeAssigned: %d/%d, assignMap: %v\n", g.CurrentTime(), g.getAverageAssignedThread(g.SMAssignedThreadTable), nCTAToBeAssigned, maxCandidates, assignMap)
			if nCTAToBeAssigned == 0 {
				// couldn't assign any CTA now
				return true
			}

			// send assigned CTAs
			for tIdx := 0; tIdx < nCTAToBeAssigned; tIdx++ {
				if len(g.undispatchedThreadblocks) == 0 {
					break
				}
				smIndex := assignMap[tIdx]
				sm := g.SMList[smIndex]

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
				// remove the head CTA (we always take from front)
				g.undispatchedThreadblocks = g.undispatchedThreadblocks[1:]
			}

			// advance issue index a bit to rotate start point
			g.SMIssueIndex = (g.SMIssueIndex + uint64(nCTAToBeAssigned)) % g.smsCount // g.SMIssueIndex = (g.SMIssueIndex + uint64(nCTAToBeAssigned)) % g.smsCount
		}
	}

	g.GPU2SMThreadBlockAllocationLatencyRemaining = g.GPU2SMThreadBlockAllocationLatency
	// }
	return false
}

func (g *GPUController) LogStatus() {
}
