package emu

import (
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// InstEmuState is the interface used by the emulator to track the instruction
// execution status.
type InstEmuState interface {
	PID() vm.PID
	Inst() *insts.Inst

	ReadOperand(operand *insts.Operand, laneID int) uint64
	WriteOperand(operand *insts.Operand, laneID int, value uint64)
	ReadOperandBytes(operand *insts.Operand, laneID int, byteCount int) []byte
	WriteOperandBytes(operand *insts.Operand, laneID int, data []byte)

	EXEC() uint64
	SetEXEC(v uint64)
	VCC() uint64
	SetVCC(v uint64)
	SCC() byte
	SetSCC(v byte)
	PC() uint64
	SetPC(v uint64)
}
