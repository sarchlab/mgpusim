package cu

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A RegReadReq is a request to read a set of register
type RegReadReq struct {
	*core.ReqBase

	Reg      *insts.Reg
	ByteSize int
	Buf      []byte
}

// A WriteReadReq is a request to read a set of register
type WriteReadReq struct {
	*core.ReqBase

	Reg *insts.Reg
	Buf []byte
}

// A RegCtrl is a Yaotsu component that is responsible for the
// timing of reading and writing registers.
//
//     <=> ToOutside the only port that the RegCtrl use to connect to the
//         outside world.
type RegCtrl struct {
	*core.BasicComponent

	storage *mem.Storage
}
