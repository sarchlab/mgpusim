package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A ScalarUnit performs Scalar operations
type ScalarUnit struct {
	cu *ComputeUnit

	alu emu.ALU

	readBufSize int

	log2CachelineSize uint64
}

// NewScalarUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewScalarUnit(
	cu *ComputeUnit,
	alu emu.ALU,
) *ScalarUnit {
	u := new(ScalarUnit)
	u.cu = cu
	u.alu = alu
	u.readBufSize = 16
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *ScalarUnit) CanAcceptWave() bool {
	return u.state().Stages.Read == 0
}

// IsIdle checks idleness
func (u *ScalarUnit) IsIdle() bool {
	u.state().Stages.Idle = (u.state().Stages.Read == 0) && (u.state().Stages.Write == 0) &&
		(u.state().Stages.Exec == 0) && (len(u.state().Reads) == 0)
	return u.state().Stages.Idle
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *ScalarUnit) AcceptWave(wave *Wavefront) {
	u.state().Stages.Read = u.cu.useWave(wave)
	inst := wave.Inst()
	dynInst := wave.DynamicInst()
	if inst != nil && dynInst != nil && inst.FormatType == insts.SMEM {
		u.startIssueSubtask(dynInst)
	}
}

func (u *ScalarUnit) startIssueSubtask(inst *wavefront.Inst) {
	taskID := u.cu.comp.TaskTracker().StartTask(u.cu.comp, tasks.StartInfo{
		ParentID: u.cu.comp.State.InstTasks[inst.ID],
		Kind:     "pipeline",
		What:     u.cu.comp.Name() + ".smem_issue",
	})
	u.state().Tasks[inst.ID] = taskID
}

func (u *ScalarUnit) endIssueSubtask(inst *wavefront.Inst) {
	taskID, ok := u.state().Tasks[inst.ID]
	if !ok {
		return
	}

	u.cu.comp.TaskTracker().EndTask(taskID)
	u.cu.comp.TaskTracker().AddMilestone(tasks.Milestone{
		TaskID: u.cu.comp.State.InstTasks[inst.ID],
		Kind:   tasks.MilestoneKindWork,
		What:   u.cu.comp.Name() + ".smem_issue",
	})
	delete(u.state().Tasks, inst.ID)
}

// Run executes three pipeline stages that are controlled by the ScalarUnit
func (u *ScalarUnit) Run() bool {
	madeProgress := false
	madeProgress = u.sendRequest() || madeProgress
	madeProgress = u.runWriteStage() || madeProgress
	madeProgress = u.runExecStage() || madeProgress
	madeProgress = u.runReadStage() || madeProgress
	return madeProgress
}

func (u *ScalarUnit) runReadStage() bool {
	if u.state().Stages.Read == 0 {
		return false
	}

	if u.state().Stages.Exec == 0 {
		u.state().Stages.Exec = u.state().Stages.Read
		u.state().Stages.Read = 0
		return true
	}
	return false
}

func (u *ScalarUnit) runExecStage() bool {
	if u.state().Stages.Exec == 0 {
		return false
	}
	if u.state().Stages.Write == 0 {
		if u.cu.wave(u.state().Stages.Exec).Inst().FormatType == insts.SMEM {
			u.executeSMEMInst()
			return true
		}

		u.alu.Run(u.cu.wave(u.state().Stages.Exec).emulation())

		u.state().Stages.Write = u.state().Stages.Exec
		u.state().Stages.Exec = 0

		return true
	}
	return false
}

func (u *ScalarUnit) executeSMEMInst() bool {
	inst := u.cu.wave(u.state().Stages.Exec).Inst()
	switch inst.Opcode {
	case 0:
		return u.executeSMEMLoad(4)
	case 1:
		return u.executeSMEMLoad(8)
	case 2:
		return u.executeSMEMLoad(16)
	case 3:
		return u.executeSMEMLoad(32)
	default:
		log.Panicf("opcode %d is not supported.", inst.Opcode)
	}

	panic("never")
}

func (u *ScalarUnit) executeSMEMLoad(byteSize int) bool {
	inst := u.cu.wave(u.state().Stages.Exec).DynamicInst()
	rawInst := u.cu.wave(u.state().Stages.Exec).Inst()
	baseVal := u.cu.wave(u.state().Stages.Exec).ReadOperand(rawInst.Base, 0)
	offsetVal := u.cu.wave(u.state().Stages.Exec).ReadOperand(rawInst.Offset, 0)
	start := baseVal + offsetVal
	numCacheline := u.numCacheline(start, uint64(byteSize))

	if len(u.state().Reads)+numCacheline > u.readBufSize {
		return false
	}

	curr := start
	bytesLeft := uint64(byteSize)
	regIndex := inst.Data.Register.RegIndex()
	for bytesLeft > 0 {
		bytesLeftInCacheline := u.byteInCacheline(curr, bytesLeft)
		bytesLeft -= bytesLeftInCacheline

		req := messaging.Msg{ID: u.cu.comp.NewID(),
			Src: u.cu.scalarMemPort().AsRemote(),
			Dst: u.cu.comp.Spec.ScalarMem, Payload: memprotocol.ReadReq{Address: curr,
				AccessByteSize: bytesLeftInCacheline,
				PID:            u.cu.wave(u.state().Stages.Exec).PID()}}

		u.state().Reads = append(u.state().Reads, req)

		info := &ScalarMemAccessInfo{
			Req:       req,
			Wavefront: u.cu.wave(u.state().Stages.Exec),
			DstSGPR:   insts.SReg(regIndex + int((curr-start)/4)),
			Inst:      inst,
		}
		u.cu.comp.State.Scalar = append(
			u.cu.comp.State.Scalar, u.cu.scalarState(info))

		u.cu.startRequest(req, u.cu.comp.State.InstTasks[u.cu.wave(u.state().Stages.Exec).DynamicInst().ID])

		curr += bytesLeftInCacheline
	}

	u.endIssueSubtask(inst)

	u.cu.wave(u.state().Stages.Exec).execution().OutstandingScalarMemAccess++
	u.cu.UpdatePCAndSetReady(u.cu.wave(u.state().Stages.Exec))
	u.state().Stages.Exec = 0

	return true
}

func (u ScalarUnit) numCacheline(start, byteSize uint64) int {
	count := 1
	curr := start
	cachelineSize := uint64(1) << u.log2CachelineSize
	mask := ^(uint64(1<<u.log2CachelineSize) - 1)

	for byteSize > 0 {
		cachelineStart := curr & mask
		cachelineEnd := cachelineStart + cachelineSize
		bytesLeftInCacheline := cachelineEnd - curr
		if byteSize <= bytesLeftInCacheline {
			return count
		}
		count++
		curr += bytesLeftInCacheline
		byteSize -= bytesLeftInCacheline
	}

	return count
}

func (u ScalarUnit) byteInCacheline(curr, bytesLeft uint64) uint64 {
	mask := ^(uint64(1<<u.log2CachelineSize) - 1)
	cachelineStart := curr & mask
	cachelineEnd := cachelineStart + (1 << u.log2CachelineSize)

	bytesLeftInCacheline := cachelineEnd - curr
	if bytesLeftInCacheline < bytesLeft {
		return bytesLeftInCacheline
	}

	return bytesLeft
}

func (u *ScalarUnit) runWriteStage() bool {
	if u.state().Stages.Write == 0 {
		return false
	}

	u.cu.logInstTask(u.cu.wave(u.state().Stages.Write), u.cu.wave(u.state().Stages.Write).DynamicInst(), true)

	u.cu.UpdatePCAndSetReady(u.cu.wave(u.state().Stages.Write))

	u.state().Stages.Write = 0
	return true
}

func (u *ScalarUnit) sendRequest() bool {
	madeProgress := false
	for i := 0; i < 4 && len(u.state().Reads) > 0; i++ {
		req := u.state().Reads[0]
		if !u.cu.scalarMemPort().CanSend() {
			break
		}

		u.cu.scalarMemPort().Send(req)
		u.state().Reads = u.state().Reads[1:]
		madeProgress = true
	}
	return madeProgress
}

func (u *ScalarUnit) state() *ScalarUnitState { return &u.cu.comp.State.ScalarUnit }
