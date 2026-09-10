package cu

import (
	"log"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// A RegisterAccess is an incidence of reading or writing the register
type RegisterAccess struct {
	Time       timing.VTimeInPicoSec
	Reg        *insts.Reg
	RegCount   int
	LaneID     int
	WaveOffset int
	Data       []byte
	OK         bool
}

// A RegisterFile provides the communication interface for a set of registers.
type RegisterFile interface {
	Read(access RegisterAccess)
	Write(access RegisterAccess)
}

// A SimpleRegisterFile is a Register file that can always read and write
// registers immediately
type SimpleRegisterFile struct {
	storage []byte

	// ByteSizePerLane is the physical VGPR capacity assigned to one lane.
	// It must track the total per-SIMD VGPR capacity: 1024 bytes for 16384
	// VGPRs and 2048 bytes for 32768 VGPRs across a 64-lane wavefront.
	ByteSizePerLane int
}

// NewSimpleRegisterFile creates and returns a new SimpleRegisterFile
func NewSimpleRegisterFile(
	byteSize uint64,
	byteSizePerLane int,
) *SimpleRegisterFile {
	r := new(SimpleRegisterFile)
	r.storage = make([]byte, byteSize)
	r.ByteSizePerLane = byteSizePerLane
	return r
}

func (r *SimpleRegisterFile) Write(access RegisterAccess) {
	offset := r.getRegOffset(access.Reg, access.WaveOffset, access.LaneID)

	if access.RegCount == 0 {
		access.RegCount = 1
	}

	size := access.RegCount * 4
	copy(r.storage[offset:offset+size], access.Data[0:access.RegCount*4])
	access.OK = true
}

func (r *SimpleRegisterFile) Read(access RegisterAccess) {
	offset := r.getRegOffset(access.Reg, access.WaveOffset, access.LaneID)

	if access.RegCount == 0 {
		access.RegCount = 1
	}

	size := access.RegCount * 4
	copy(access.Data, r.storage[offset:offset+size])
	access.OK = true
}

func (r *SimpleRegisterFile) getRegOffset(reg *insts.Reg, offset int, laneID int) int {
	if reg.IsSReg() {
		return reg.RegIndex()*4 + offset
	}

	if reg.IsVReg() {
		regOffset := reg.RegIndex()*4 + laneID*r.ByteSizePerLane + offset
		return regOffset
	}

	log.Panic("Register type not supported by register files")

	return 0
}
