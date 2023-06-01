package emu

import (
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/mgpusim/v3/insts"
)

// InstEmuState is the interface used by the emulator to track the instruction
// execution status.
type InstEmuState interface {
	PID() vm.PID
	Inst() *insts.Inst
	Scratchpad() Scratchpad
}
