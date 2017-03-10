package gcn3

import (
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/gcn3/disasm"
)

// A ComputeUnit is where the GPU kernel is executed, in the unit of work group.
type ComputeUnit interface {
	conn.Component

	WriteReg(reg *disasm.Reg, wiFlatID int, data []byte)
	ReadReg(reg *disasm.Reg, wiFlatID int, byteSize int) []byte
}
