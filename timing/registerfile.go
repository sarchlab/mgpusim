package timing

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A RegisterAccess is an incidence of reading or writing the register
type RegisterAccess struct {
	Time       core.VTimeInSec
	Reg        *insts.Reg
	LaneID     int
	WaveOffset int
	Data       []byte
	OK         bool
}

// A RegisterFile provides the communication interface for a set of registers.
type RegisterFile interface {
	Read(access *RegisterAccess)
	Write(access *RegisterAccess)
}

// A SimpleRegisterFile is a Register file that can always read and write
// registers immediately
type SimpleRegisterFile struct {
	storage *mem.Storage
}

// NewSimpleRegisterFile creates and returns a new SimpleRegisterFile
func NewSimpleRegisterFile(byteSize uint64) *SimpleRegisterFile {
	r := new(SimpleRegisterFile)
	r.storage = mem.NewStorage(byteSize)
	return r
}

func (r *SimpleRegisterFile) Write(access *RegisterAccess) {
}

func (r *SimpleRegisterFile) Read(access *RegisterAccess) {
}
