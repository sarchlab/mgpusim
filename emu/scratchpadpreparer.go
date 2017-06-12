package emu

import (
	"log"

	"gitlab.com/yaotsu/gcn3/insts"
)

// ScratchpadPreparer is the unit that sets the instruction scratchpad
// before the instruction can be emulated.
type ScratchpadPreparer interface {
	// Prepare reads from the register file and write into the instruction
	// scratchpad
	Prepare(instEmuState InstEmuState, wf interface{})

	// Commit write to the register file to reflect the change in the scratchpad
	Commit(instEmuState InstEmuState, wf interface{})
}

// ScratchpadPreparerImpl provides a standard implmentation of the
// ScratchpadPreparer
type ScratchpadPreparerImpl struct {
	regInterface RegInterface
}

// NewScratchpadPreparerImpl returns a newly created ScratchpadPreparerImpl,
// injecting the dependency of the RegInterface.
func NewScratchpadPreparerImpl(regInterface RegInterface) *ScratchpadPreparerImpl {
	p := new(ScratchpadPreparerImpl)
	p.regInterface = regInterface
	return p
}

// Prepare read from the register file and sets the scratchpad layout
func (p *ScratchpadPreparerImpl) Prepare(
	instEmuState InstEmuState,
	wf interface{},
) {
	inst := instEmuState.Inst()
	switch inst.FormatType {
	case insts.Sop2:
		p.prepareSOP2ScratchPad(instEmuState, wf)
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}

func (p *ScratchpadPreparerImpl) prepareSOP2ScratchPad(
	instEmuState InstEmuState,
	wf interface{},
) {
	// inst := instEmuState.Inst()
	// inst.Src0.
}

// Commit write to the register file according to the scratchpad layout
func (p *ScratchpadPreparerImpl) Commit(
	instEmuState InstEmuState,
	wf interface{},
) {
	inst := instEmuState.Inst()
	switch inst.FormatType {
	default:
		log.Panicf("Inst format %s is not supported", inst.Format.FormatName)
	}
}
