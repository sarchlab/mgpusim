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
	Offset   int
	Buf      []byte
}

// NewReadReq returns a newly create RegReadReq
func NewReadReq(reg *inst.Reg, ByteSize int) *RegReadReq {
}

// A WriteReadReq is a request to read a set of register
type WriteReadReq struct {
	*core.ReqBase

	Reg    *insts.Reg
	Offset int
	Buf    []byte
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

// NewRegCtrl returns a newly created RegCtrl
func NewRegCtrl(name string, byteSize uint64) *RegCtrl {
	c := new(RegCtrl)
	c.BasicComponent = core.NewBasicComponent(name)
	c.storage = mem.NewStorage(byteSize)
	return c
}

// Recv processes incomming requests
func (c *RegCtrl) Recv(req core.Req) *core.Error {
	return nil
}

// Handle processes the event that is scheduled on the RegCtrl
func (c *RegCtrl) Handle(evt core.Event) error {
	return nil
}
