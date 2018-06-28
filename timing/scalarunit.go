package timing

import (
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A ScalarUnit performs Scalar operations
type ScalarUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU

	toRead  *Wavefront
	toExec  *Wavefront
	toWrite *Wavefront

	readBufSize int
	readBuf     []*mem.ReadReq
}

// NewScalarUnit creates a new Scalar unit, injecting the dependency of
// the compute unit.
func NewScalarUnit(
	cu *ComputeUnit,
	scratchpadPreparer ScratchpadPreparer,
	alu emu.ALU,
) *ScalarUnit {
	u := new(ScalarUnit)
	u.cu = cu
	u.scratchpadPreparer = scratchpadPreparer
	u.alu = alu
	u.readBufSize = 16
	u.readBuf = make([]*mem.ReadReq, 0, u.readBufSize)
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *ScalarUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *ScalarUnit) AcceptWave(wave *Wavefront, now core.VTimeInSec) {
	u.toRead = wave
	u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ReadStart"})
}

// Run executes three pipeline stages that are controlled by the ScalarUnit
func (u *ScalarUnit) Run(now core.VTimeInSec) {
	u.sendRequest(now)
	u.runWriteStage(now)
	u.runExecStage(now)
	u.runReadStage(now)
}

func (u *ScalarUnit) runReadStage(now core.VTimeInSec) {
	if u.toRead == nil {
		return
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ReadEnd"})
		u.cu.InvokeHook(u.toRead, u.cu, core.Any, &InstHookInfo{now, u.toRead.inst, "ExecStart"})

		u.toExec = u.toRead
		u.toRead = nil
	}
}

func (u *ScalarUnit) runExecStage(now core.VTimeInSec) {
	if u.toExec == nil {
		return
	}

	if u.toWrite == nil {
		if u.toExec.Inst().FormatType == insts.SMEM {
			u.executeSMEMInst(now)
			return
		} else {
			u.alu.Run(u.toExec)
			u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "ExecEnd"})
			u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "WriteStart"})
			u.toWrite = u.toExec
			u.toExec = nil
		}
	}
}

func (u *ScalarUnit) executeSMEMInst(now core.VTimeInSec) {
	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 0:
		u.executeSMEMLoad(4, now)
	case 1:
		u.executeSMEMLoad(8, now)
	default:
		log.Panicf("opcode %d is not supported.", inst.Opcode)
	}
}

func (u *ScalarUnit) executeSMEMLoad(byteSize int, now core.VTimeInSec) {
	inst := u.toExec.inst
	sp := u.toExec.Scratchpad().AsSMEM()

	//u.cu.GetConnection("ToScalarMem").Send(req)
	if len(u.readBuf) < u.readBufSize {
		u.toExec.OutstandingScalarMemAccess += 1

		req := mem.NewReadReq(now, u.cu, u.cu.ScalarMem,
			sp.Base+sp.Offset, uint64(byteSize))
		u.readBuf = append(u.readBuf, req)

		info := newMemAccessInfo()
		info.Wf = u.toExec
		info.Action = MemAccessScalarDataLoad
		info.Dst = inst.Data.Register
		info.Inst = inst
		u.cu.inFlightMemAccess[req.ID] = info

		u.toExec.State = WfReady
		u.cu.InvokeHook(u.toExec, u.cu, core.Any, &InstHookInfo{now, u.toExec.inst, "WaitMem"})
		u.toExec = nil
	}
}

func (u *ScalarUnit) runWriteStage(now core.VTimeInSec) {
	if u.toWrite == nil {
		return
	}

	u.scratchpadPreparer.Commit(u.toWrite, u.toWrite)

	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, u.toWrite.inst, "WriteEnd"})
	u.cu.InvokeHook(u.toWrite, u.cu, core.Any, &InstHookInfo{now, u.toWrite.inst, "Completed"})

	u.toWrite.State = WfReady
	u.toWrite = nil
}

func (u *ScalarUnit) sendRequest(now core.VTimeInSec) {
	if len(u.readBuf) > 0 {
		req := u.readBuf[0]
		req.SetSendTime(now)
		err := u.cu.GetConnection("ToScalarMem").Send(req)
		if err == nil {
			u.readBuf = u.readBuf[1:]
		}
	}
}
