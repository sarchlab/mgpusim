package emu

import (
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/util/v2/ca"
)

// InstEmuState is the interface used by the emulator to track the instruction
// execution status.
type InstEmuState interface {
	PID() ca.PID
	Inst() *insts.Inst
	Scratchpad() Scratchpad
}
