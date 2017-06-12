package emu

import (
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// A Wavefront in the emu package is a wrapper for the kernels.Wavefront
type Wavefront struct {
	*kernels.Wavefront

	CodeObject *insts.HsaCo
	Packet     *kernels.HsaKernelDispatchPacket

	Completed  bool
	AtBarrier  bool
	Inst       *insts.Inst
	ScratchPad []byte
	PC         uint64
	SRegFile   []byte
	VRegFile   []byte
}

// NewWavefront returns the Wavefront that wraps the nativeWf
func NewWavefront(nativeWf *kernels.Wavefront) *Wavefront {
	wf := new(Wavefront)
	wf.Wavefront = nativeWf

	if nativeWf != nil {
		wf.CodeObject = nativeWf.WG.Grid.CodeObject
		wf.Packet = nativeWf.WG.Grid.Packet
	}

	wf.SRegFile = make([]byte, 4*102)
	wf.VRegFile = make([]byte, 4*64*100)

	return wf
}
