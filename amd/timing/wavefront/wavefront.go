package wavefront

import (
	"log"
	"math"
	"sync"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// RegFileAccessor provides access to register files for a wavefront.
type RegFileAccessor interface {
	ReadReg(reg *insts.Reg, regCount int, laneID int, waveOffset int) []byte
	WriteReg(reg *insts.Reg, regCount int, laneID int, waveOffset int, data []byte)
}

// WfState marks what state that wavefront it in.
type WfState int

// A list of all possible WfState
const (
	WfDispatching      WfState = iota // Dispatching in progress, not ready to run
	WfReady                           // Allow the scheduler to schedule instruction
	WfRunning                         // Instruction in fight
	WfCompleted                       // Wavefront completed
	WfAtBarrier                       // Wavefront at barrier
	WfSampledCompleted                // Wavefront completed at Sampling
)

// A Wavefront in the timing package contains the information of the progress
// of a wavefront
type Wavefront struct {
	*kernels.Wavefront
	sync.RWMutex

	WG *WorkGroup

	pid            vm.PID
	State          WfState
	inst           *Inst          // The instruction that is being executed
	scratchpad     emu.Scratchpad // A temp data buf that is shared by different stages
	LastFetchTime  sim.VTimeInSec // The time that the last instruction was fetched
	CompletedLanes int            // The number of lanes that is completed in the SIMD unit

	InstBuffer        []byte
	InstBufferStartPC uint64
	IsFetching        bool
	InstToIssue       *Inst

	SIMDID     int
	SRegOffset int
	VRegOffset int
	LDSOffset  int

	pc   uint64
	exec uint64
	vcc  uint64
	M0   uint32
	scc  byte

	RegAccessor RegFileAccessor

	OutstandingScalarMemAccess int
	OutstandingVectorMemAccess int
}

// NewWavefront creates a new Wavefront of the timing package, wrapping the
// wavefront from the kernels package.
func NewWavefront(raw *kernels.Wavefront) *Wavefront {
	wf := new(Wavefront)
	wf.Wavefront = raw

	wf.scratchpad = make([]byte, 4096)
	wf.InstBuffer = make([]byte, 0, 256)

	return wf
}

// Inst return the instruction that is being simulated
func (wf *Wavefront) Inst() *insts.Inst {
	if wf.inst == nil {
		return nil
	}
	return wf.inst.Inst
}

// DynamicInst returns the insts with an ID
func (wf *Wavefront) DynamicInst() *Inst {
	return wf.inst
}

// SetDynamicInst sets the dynamic inst to execute
func (wf *Wavefront) SetDynamicInst(i *Inst) {
	wf.inst = i
}

// ManagedInst returns the wrapped Inst
func (wf *Wavefront) ManagedInst() *Inst {
	return wf.inst
}

// Scratchpad returns the scratchpad of the wavefront
func (wf *Wavefront) Scratchpad() emu.Scratchpad {
	return wf.scratchpad
}

// PID returns pid
func (wf *Wavefront) PID() vm.PID {
	return wf.pid
}

// SetPID sets pid
func (wf *Wavefront) SetPID(pid vm.PID) {
	wf.pid = pid
}

// PC returns the program counter
func (wf *Wavefront) PC() uint64 {
	return wf.pc
}

// SetPC sets the program counter
func (wf *Wavefront) SetPC(v uint64) {
	wf.pc = v
}

// EXEC returns the exec mask
func (wf *Wavefront) EXEC() uint64 {
	return wf.exec
}

// SetEXEC sets the exec mask
func (wf *Wavefront) SetEXEC(v uint64) {
	wf.exec = v
}

// VCC returns the vector condition code
func (wf *Wavefront) VCC() uint64 {
	return wf.vcc
}

// SetVCC sets the vector condition code
func (wf *Wavefront) SetVCC(v uint64) {
	wf.vcc = v
}

// SCC returns the scalar condition code
func (wf *Wavefront) SCC() byte {
	return wf.scc
}

// SetSCC sets the scalar condition code
func (wf *Wavefront) SetSCC(v byte) {
	wf.scc = v
}

// ReadOperand reads the value of an operand using the RegFileAccessor
func (wf *Wavefront) ReadOperand(operand *insts.Operand, laneID int) uint64 {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.VRegOffset
		}
		buf := wf.RegAccessor.ReadReg(operand.Register, operand.RegCount, laneID, waveOffset)
		return insts.BytesToUint64(buf)
	case insts.IntOperand:
		return uint64(operand.IntValue)
	case insts.FloatOperand:
		return uint64(math.Float32bits(float32(operand.FloatValue)))
	case insts.LiteralConstant:
		return uint64(operand.LiteralConstant)
	default:
		log.Panicf("Unsupported operand type: %s", operand.String())
		return 0
	}
}

// WriteOperand writes a value to an operand using the RegFileAccessor
func (wf *Wavefront) WriteOperand(operand *insts.Operand, laneID int, value uint64) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	numBytes := operand.Register.ByteSize
	if operand.RegCount >= 2 {
		numBytes *= operand.RegCount
	}

	waveOffset := wf.SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.VRegOffset
	}

	data := insts.Uint64ToBytes(value)
	wf.RegAccessor.WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data[:numBytes])
}

// ReadOperandBytes reads the raw bytes of an operand
func (wf *Wavefront) ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte {
	switch operand.OperandType {
	case insts.RegOperand:
		waveOffset := wf.SRegOffset
		if operand.Register.IsVReg() {
			waveOffset = wf.VRegOffset
		}
		buf := wf.RegAccessor.ReadReg(operand.Register, operand.RegCount, laneID, waveOffset)
		if len(buf) > byteCount {
			return buf[:byteCount]
		}
		return buf
	case insts.IntOperand:
		data := insts.Uint64ToBytes(uint64(operand.IntValue))
		return data[:byteCount]
	case insts.FloatOperand:
		data := insts.Uint64ToBytes(uint64(math.Float32bits(float32(operand.FloatValue))))
		return data[:byteCount]
	case insts.LiteralConstant:
		data := insts.Uint64ToBytes(uint64(operand.LiteralConstant))
		return data[:byteCount]
	default:
		log.Panicf("Unsupported operand type: %s", operand.String())
		return nil
	}
}

// WriteOperandBytes writes raw bytes to an operand
func (wf *Wavefront) WriteOperandBytes(operand *insts.Operand, laneID int, data []byte) {
	if operand.OperandType != insts.RegOperand {
		log.Panicf("Cannot write to non-register operand: %s", operand.String())
	}

	waveOffset := wf.SRegOffset
	if operand.Register.IsVReg() {
		waveOffset = wf.VRegOffset
	}

	wf.RegAccessor.WriteReg(operand.Register, operand.RegCount, laneID, waveOffset, data)
}
