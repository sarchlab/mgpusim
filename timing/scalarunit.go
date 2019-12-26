package timing

import (
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/emu"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/timing/wavefront"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util/tracing"
)

// A ScalarUnit performs Scalar operations
type ScalarUnit struct {
	cu *ComputeUnit

	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU

	toRead  *wavefront.Wavefront
	toExec  *wavefront.Wavefront
	toWrite *wavefront.Wavefront

	readBufSize int
	readBuf     []*mem.ReadReq

	isIdle bool
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

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *ScalarUnit) IsIdle() bool {
	u.isIdle = (u.toRead == nil) && (u.toWrite == nil) && (u.toExec == nil) && (len(u.readBuf) == 0)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *ScalarUnit) AcceptWave(wave *wavefront.Wavefront, now akita.VTimeInSec) {
	u.toRead = wave
}

// Run executes three pipeline stages that are controlled by the ScalarUnit
func (u *ScalarUnit) Run(now akita.VTimeInSec) bool {
	madeProgress := false
	madeProgress = u.sendRequest(now) || madeProgress
	madeProgress = u.runWriteStage(now) || madeProgress
	madeProgress = u.runExecStage(now) || madeProgress
	madeProgress = u.runReadStage(now) || madeProgress
	return madeProgress
}

func (u *ScalarUnit) runReadStage(now akita.VTimeInSec) bool {
	if u.toRead == nil {
		return false
	}

	if u.toExec == nil {
		u.scratchpadPreparer.Prepare(u.toRead, u.toRead)

		u.toExec = u.toRead
		u.toRead = nil
		return true
	}
	return false
}

func (u *ScalarUnit) runExecStage(now akita.VTimeInSec) bool {
	if u.toExec == nil {
		return false
	}

	if u.toWrite == nil {
		if u.toExec.Inst().FormatType == insts.SMEM {
			u.executeSMEMInst(now)
			return true
		} else {
			u.alu.Run(u.toExec)

			u.toWrite = u.toExec
			u.toExec = nil
		}
		return true
	}
	return false
}

func (u *ScalarUnit) executeSMEMInst(now akita.VTimeInSec) {
	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 0:
		u.executeSMEMLoad(4, now)
	case 1:
		u.executeSMEMLoad(8, now)
	case 2:
		u.executeSMEMLoad(16, now)
	default:
		log.Panicf("opcode %d is not supported.", inst.Opcode)
	}
}

func (u *ScalarUnit) executeSMEMLoad(byteSize int, now akita.VTimeInSec) {
	inst := u.toExec.DynamicInst()
	sp := u.toExec.Scratchpad().AsSMEM()

	if len(u.readBuf) < u.readBufSize {
		u.toExec.OutstandingScalarMemAccess++

		req := mem.ReadReqBuilder{}.
			WithSendTime(now).
			WithSrc(u.cu.ToScalarMem).
			WithDst(u.cu.ScalarMem).
			WithAddress(sp.Base + sp.Offset).
			WithPID(u.toExec.PID()).
			WithByteSize(uint64(byteSize)).
			Build()
		u.readBuf = append(u.readBuf, req)

		info := new(ScalarMemAccessInfo)
		info.Req = req
		info.Wavefront = u.toExec
		info.DstSGPR = inst.Data.Register
		info.Inst = inst
		u.cu.InFlightScalarMemAccess = append(u.cu.InFlightScalarMemAccess,
			info)

		u.cu.UpdatePCAndSetReady(u.toExec)

		tracing.TraceReqInitiate(req, now, u.cu, u.toExec.DynamicInst().ID)

		u.toExec = nil
	}
}

func (u *ScalarUnit) runWriteStage(now akita.VTimeInSec) bool {
	if u.toWrite == nil {
		return false
	}

	u.scratchpadPreparer.Commit(u.toWrite, u.toWrite)

	u.cu.logInstTask(now, u.toWrite, u.toWrite.DynamicInst(), true)

	u.cu.UpdatePCAndSetReady(u.toWrite)

	u.toWrite = nil
	return true
}

func (u *ScalarUnit) sendRequest(now akita.VTimeInSec) bool {
	if len(u.readBuf) > 0 {
		req := u.readBuf[0]
		req.SendTime = now
		err := u.cu.ToScalarMem.Send(req)
		if err == nil {
			u.readBuf = u.readBuf[1:]
			return true
		}
	}
	return false
}

func (u *ScalarUnit) Flush() {
	u.toRead = nil
	u.toExec = nil
	u.toWrite = nil
	u.readBuf = nil
}
