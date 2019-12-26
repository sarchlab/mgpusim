package emu

import "gitlab.com/akita/mgpusim/insts"

// Decoder defines the interface that can convert instruction bytes to
// instructions
type Decoder interface {
	Decode(buf []byte) (*insts.Inst, error)
}
