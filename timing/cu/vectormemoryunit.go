package cu

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/timing/wavefront"
	"gitlab.com/akita/util/tracing"
)

// A VectorMemoryUnit performs Scalar operations
type VectorMemoryUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	coalescer          coalescer

	SendBuf     []mem.AccessReq
	SendBufSize int

	AddrCoalescingLatency int

	toRead                  []*wavefront.Wavefront
	toExec                  []*wavefront.Wavefront
	toWrite                 []*wavefront.Wavefront
	AddrCoalescingCycleLeft map[string]int

	toRemoveFromExecStage []int

	instructionsInFlight    uint64
	maxInstructionsInFlight uint64

	isIdle bool
}

// NewVectorMemoryUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewVectorMemoryUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	coalescer coalescer,
) *VectorMemoryUnit {
	u := new(VectorMemoryUnit)
	u.cu = cu

	u.scratchpadPreparer = scratchpadPreparer
	u.coalescer = coalescer

	u.SendBufSize = 256
	u.SendBuf = make([]mem.AccessReq, 0, u.SendBufSize)

	u.AddrCoalescingLatency = 130
	u.maxInstructionsInFlight = 130
	u.AddrCoalescingCycleLeft = make(map[string]int)

	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *VectorMemoryUnit) CanAcceptWave() bool {
	if u.instructionsInFlight >= u.maxInstructionsInFlight {
		return false
	}
	return true
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) AcceptWave(wave *wavefront.Wavefront, now akita.VTimeInSec) {
	u.toRead = append(u.toRead, wave)
	u.instructionsInFlight++
}

// IsIdle moves one wavefront into the read buffer of the Scalar unit
func (u *VectorMemoryUnit) IsIdle() bool {
	u.isIdle = (len(u.toRead) == 0) && (len(u.toExec) == 0) && (len(u.toWrite) == 0) && (len(u.SendBuf) == 0)
	return u.isIdle
}

// Run executes three pipeline stages that are controlled by the VectorMemoryUnit
func (u *VectorMemoryUnit) Run(now akita.VTimeInSec) bool {
	madeProgress := false
	madeProgress = madeProgress || u.sendRequest(now)
	madeProgress = madeProgress || u.runExecStage(now)
	madeProgress = madeProgress || u.runReadStage(now)
	return madeProgress
}

func (u *VectorMemoryUnit) runReadStage(now akita.VTimeInSec) bool {
	if len(u.toRead) == 0 {
		return false
	}

	for i := 0; i < len(u.toRead); i++ {
		u.scratchpadPreparer.Prepare(u.toRead[i], u.toRead[i])
		u.toExec = append(u.toExec, u.toRead[i])
		u.AddrCoalescingCycleLeft[u.toRead[i].UID] = u.AddrCoalescingLatency
	}
	u.toRead = nil
	return true
}

func (u *VectorMemoryUnit) runExecStage(now akita.VTimeInSec) bool {
	if len(u.toExec) == 0 {
		return false
	}

	for i := 0; i < len(u.toExec); i++ {
		waveID := u.toExec[i].UID
		if u.AddrCoalescingCycleLeft[waveID] > 0 {
			u.AddrCoalescingCycleLeft[waveID]--
		} else {
			inst := u.toExec[i].Inst()
			switch inst.FormatType {
			case insts.FLAT:
				u.executeFlatInsts(now, i)
			default:
				log.Panicf("running inst %s in vector memory unit is not supported", inst.String(nil))
			}
			u.cu.UpdatePCAndSetReady(u.toExec[i])
			delete(u.AddrCoalescingCycleLeft, u.toExec[i].UID)
			u.instructionsInFlight--
			u.toRemoveFromExecStage = append(u.toRemoveFromExecStage, i)
		}
	}

	tmp := u.toExec[:0]

	for i := 0; i < len(u.toExec); i++ {
		if !u.isRemovedFromExecStage(i) {
			tmp = append(tmp, u.toExec[i])
		}
	}

	u.toExec = tmp
	u.toRemoveFromExecStage = nil

	return true
}

func (u *VectorMemoryUnit) isRemovedFromExecStage(index int) bool {
	for i := 0; i < len(u.toRemoveFromExecStage); i++ {
		remove := u.toRemoveFromExecStage[i]
		if remove == index {
			return true
		}
	}
	return false
}

func (u *VectorMemoryUnit) executeFlatInsts(now akita.VTimeInSec, index int) {
	inst := u.toExec[index].Inst()
	switch inst.Opcode {
	case 16, 17, 18, 19, 20, 21, 22, 23: // FLAT_LOAD_BYTE
		u.executeFlatLoad(now, index)
	case 24, 25, 26, 27, 28, 29, 30, 31:
		u.executeFlatStore(now, index)
	default:
		log.Panicf("Opcode %d for format FLAT is not supported.", inst.Opcode)
	}
}

func (u *VectorMemoryUnit) executeFlatLoad(
	now akita.VTimeInSec,
	index int,
) {
	transactions := u.coalescer.generateMemTransactions(u.toExec[index])

	if len(transactions) == 0 {
		u.cu.logInstTask(now, u.toExec[index], u.toExec[index].DynamicInst(), true)
		return
	}

	u.toExec[index].OutstandingVectorMemAccess++
	u.toExec[index].OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i != len(transactions)-1 {
			t.Read.CanWaitForCoalesce = true
		}
		lowModule := u.cu.VectorMemModules.Find(t.Read.Address)
		t.Read.Dst = lowModule
		t.Read.Src = u.cu.ToVectorMem
		t.Read.PID = u.toExec[index].PID()
		u.SendBuf = append(u.SendBuf, t.Read)
		tracing.TraceReqInitiate(t.Read, now, u.cu, u.toExec[index].DynamicInst().ID)
	}
}

func (u *VectorMemoryUnit) executeFlatStore(
	now akita.VTimeInSec,
	index int,
) {
	transactions := u.coalescer.generateMemTransactions(u.toExec[index])

	if len(transactions) == 0 {
		u.cu.logInstTask(now, u.toExec[index], u.toExec[index].DynamicInst(), true)
		return
	}

	u.toExec[index].OutstandingVectorMemAccess++
	u.toExec[index].OutstandingScalarMemAccess++

	for i, t := range transactions {
		u.cu.InFlightVectorMemAccess = append(u.cu.InFlightVectorMemAccess, t)
		if i != len(transactions)-1 {
			t.Write.CanWaitForCoalesce = true
		}
		lowModule := u.cu.VectorMemModules.Find(t.Write.Address)
		t.Write.Dst = lowModule
		t.Write.Src = u.cu.ToVectorMem
		t.Write.PID = u.toExec[index].PID()
		u.SendBuf = append(u.SendBuf, t.Write)
		tracing.TraceReqInitiate(t.Write, now, u.cu, u.toExec[index].DynamicInst().ID)
	}
}

func (u *VectorMemoryUnit) sendRequest(now akita.VTimeInSec) bool {
	madeProgress := false

	if len(u.SendBuf) > 0 {
		req := u.SendBuf[0]
		req.Meta().SendTime = now
		err := u.cu.ToVectorMem.Send(req)
		if err == nil {
			u.SendBuf = u.SendBuf[1:]
			madeProgress = true
		}
	}

	return madeProgress
}

// Flush flushes
func (u *VectorMemoryUnit) Flush() {
	for waveID := range u.AddrCoalescingCycleLeft {
		delete(u.AddrCoalescingCycleLeft, waveID)
	}
	u.SendBuf = nil
	u.toRead = nil
	u.toExec = nil
	u.toWrite = nil
	u.SendBuf = nil
	u.instructionsInFlight = 0
}
