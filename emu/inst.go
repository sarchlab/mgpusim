package emu

import (
	"gitlab.com/akita/mgpusim/v2/insts"
)

// InstEmuState is the interface used by the emulator to track the instruction
// execution status.
type InstEmuState interface {
	PID() vm.PID
	Inst() *insts.Inst
	Scratchpad() Scratchpad
}
