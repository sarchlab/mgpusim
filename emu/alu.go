package emu

import "log"

// ALU is where the instructions get executed.
type ALU struct {
}

// Run executes the instruction in the scatchpad of the InstEmuState
func (u *ALU) Run(state InstEmuState) {
	inst := state.Inst()
	switch inst.FormatType {
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}

}
