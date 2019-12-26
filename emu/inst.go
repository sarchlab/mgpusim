package emu

import (
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/util/ca"
)

// InstEmuState is the interface used by the emulator to track the instruction
// execution status.
type InstEmuState interface {
	PID() ca.PID
	Inst() *insts.Inst
	Scratchpad() Scratchpad
}
