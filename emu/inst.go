package emu

import (
	"gitlab.com/akita/gcn3/insts"
)

// InstEmuState is the interface used by the emulator to track the instuction
// execution status.
type InstEmuState interface {
	Inst() *insts.Inst
	Scratchpad() Scratchpad
}
