package emu

import (
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/mem/vm"
)

// InstEmuState is the interface used by the emulator to track the instuction
// execution status.
type InstEmuState interface {
	PID() vm.PID
	Inst() *insts.Inst
	Scratchpad() Scratchpad
}
