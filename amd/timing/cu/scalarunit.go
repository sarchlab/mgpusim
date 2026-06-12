package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

// A ScalarUnit performs Scalar operations
type ScalarUnit struct {
	cu *ComputeUnit

	alu emu.ALU

	toRead  *wavefront.Wavefront
	toExec  *wavefront.Wavefront
	toWrite *wavefront.Wavefront

	readBufSize int
	readBuf     []*mem.ReadReq

	log2CachelineSize uint64

	isIdle bool
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
	u.readBuf = make([]*mem.ReadReq, 0, u.readBufSize)
	return u
}

// CanAcceptWave checks if the buffer of the read stage is occupied or not
func (u *ScalarUnit) CanAcceptWave() bool {
	return u.toRead == nil
}

// IsIdle checks idleness
func (u *ScalarUnit) IsIdle() bool {
	u.isIdle = (u.toRead == nil) && (u.toWrite == nil) && (u.toExec == nil) && (len(u.readBuf) == 0)
	return u.isIdle
}

// AcceptWave moves one wavefront into the read buffer of the Scalar unit
func (u *ScalarUnit) AcceptWave(wave *wavefront.Wavefront) {
	u.toRead = wave
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
	if u.toRead == nil {
		return false
	}

	if u.toExec == nil {
		u.toExec = u.toRead
		u.toRead = nil
		return true
	}
	return false
}

func (u *ScalarUnit) runExecStage() bool {
	if u.toExec == nil {
		return false
	}
	if u.toWrite == nil {
		if u.toExec.Inst().FormatType == insts.SMEM {
			u.executeSMEMInst()
			return true
		}

		// Pre-advance the PC before running the ALU emulation.
		// The CDNA3 ALU expects the wavefront PC to already point
		// to the next instruction (matching the emulation path where
		// PC is advanced before execution). This is critical for
		// s_getpc_b64, which writes the current PC to a register
		// pair — the hardware returns the address of the *next*
		// instruction, not the current one.
		//
		// After the ALU runs, we restore the PC so that
		// UpdatePCAndSetReady (called in the write stage) can
		// advance it in the normal way.
		savedPC := u.toExec.PC()
		u.toExec.SetPC(savedPC + uint64(u.toExec.Inst().ByteSize))
		u.alu.Run(u.toExec)
		u.toExec.SetPC(savedPC)

		u.toWrite = u.toExec
		u.toExec = nil

		return true
	}
	return false
}

func (u *ScalarUnit) executeSMEMInst() bool {
	inst := u.toExec.Inst()
	switch inst.Opcode {
	case 0:
		return u.executeSMEMLoad(4)
	case 1:
		return u.executeSMEMLoad(8)
	case 2:
		return u.executeSMEMLoad(16)
	case 3:
		return u.executeSMEMLoad(32)
	case 4:
		return u.executeSMEMLoad(64)
	// s_buffer_load variants use the same load path
	case 8:
		return u.executeSMEMLoad(4)
	case 9:
		return u.executeSMEMLoad(8)
	case 10:
		return u.executeSMEMLoad(16)
	case 11:
		return u.executeSMEMLoad(32)
	case 12:
		return u.executeSMEMLoad(64)
	default:
		log.Panicf("opcode %d is not supported.", inst.Opcode)
	}

	panic("never")
}

func (u *ScalarUnit) executeSMEMLoad(byteSize int) bool {
	inst := u.toExec.DynamicInst()
	rawInst := u.toExec.Inst()
	baseVal := u.toExec.ReadOperand(rawInst.Base, 0)
	offsetVal := u.toExec.ReadOperand(rawInst.Offset, 0)
	start := baseVal + offsetVal
	numCacheline := u.numCacheline(start, uint64(byteSize))

	if len(u.readBuf)+numCacheline > u.readBufSize {
		return false
	}

	curr := start
	bytesLeft := uint64(byteSize)
	regIndex := inst.Data.Register.RegIndex()
	for bytesLeft > 0 {
		bytesLeftInCacheline := u.byteInCacheline(curr, bytesLeft)
		bytesLeft -= bytesLeftInCacheline

		req := &mem.ReadReq{
			Address:        curr,
			AccessByteSize: bytesLeftInCacheline,
			PID:            u.toExec.PID(),
		}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = u.cu.ToScalarMem.AsRemote()
		req.Dst = u.cu.ScalarMem.AsRemote()
		if bytesLeft > 0 {
			req.CanWaitForCoalesce = true
		}
		u.readBuf = append(u.readBuf, req)

		info := &ScalarMemAccessInfo{
			Req:       req,
			Wavefront: u.toExec,
			DstSGPR:   insts.SReg(regIndex + int((curr-start)/4)),
			Inst:      inst,
		}
		u.cu.InFlightScalarMemAccess = append(
			u.cu.InFlightScalarMemAccess, info)

		tracing.TraceReqInitiate(req, u.cu, u.toExec.DynamicInst().ID)

		curr += bytesLeftInCacheline
	}

	u.toExec.OutstandingScalarMemAccess++
	u.cu.UpdatePCAndSetReady(u.toExec)
	u.toExec = nil

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
	if u.toWrite == nil {
		return false
	}

	u.cu.logInstTask(u.toWrite, u.toWrite.DynamicInst(), true)

	u.cu.UpdatePCAndSetReady(u.toWrite)

	u.toWrite = nil
	return true
}

func (u *ScalarUnit) sendRequest() bool {
	madeProgress := false
	for i := 0; i < 4 && len(u.readBuf) > 0; i++ {
		req := u.readBuf[0]
		err := u.cu.ToScalarMem.Send(req)
		if err == nil {
			u.readBuf = u.readBuf[1:]
			madeProgress = true
		} else {
			break
		}
	}
	return madeProgress
}

// Flush clears the unit
func (u *ScalarUnit) Flush() {
	u.toRead = nil
	u.toExec = nil
	u.toWrite = nil
	u.readBuf = nil
}
