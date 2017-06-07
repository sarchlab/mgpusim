package timing

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

// A ReadRegReq is a request to read a set of register
type ReadRegReq struct {
	*core.ReqBase

	Reg       *insts.Reg
	ByteSize  int
	Offset    int
	Buf       []byte
	Completed bool // Used by the sender to mark the ACK has been received
}

// NewReadRegReq returns a newly create ReadRegReq
func NewReadRegReq(
	sendTime core.VTimeInSec,
	reg *insts.Reg,
	byteSize int,
	offset int,
) *ReadRegReq {
	r := new(ReadRegReq)

	r.ReqBase = core.NewReqBase()
	r.SetSendTime(sendTime)

	r.Reg = reg
	r.ByteSize = byteSize
	r.Offset = offset

	return r
}

// A ReadRegEvent is the event that request the register file to return
// the register value
type ReadRegEvent struct {
	*core.EventBase

	Req *ReadRegReq
}

// NewReadRegEvent returns a newly created ReadRegReq
func NewReadRegEvent(
	time core.VTimeInSec,
	handler core.Handler,
	req *ReadRegReq,
) *ReadRegEvent {
	e := new(ReadRegEvent)
	e.EventBase = core.NewEventBase(time, handler)
	e.Req = req
	return e
}

// A WriteRegReq is a request to read a set of register
type WriteRegReq struct {
	*core.ReqBase

	Reg       *insts.Reg
	Offset    int
	Buf       []byte
	Completed bool // Used by the sender to mark the ACK has been received
}

// NewWriteRegReq creates a new WriteRegReq
func NewWriteRegReq(sendTime core.VTimeInSec,
	reg *insts.Reg,
	offset int,
	data []byte,
) *WriteRegReq {
	r := new(WriteRegReq)
	r.ReqBase = core.NewReqBase()
	r.SetSendTime(sendTime)
	r.Reg = reg
	r.Offset = offset
	r.Buf = data
	return r
}

// A WriteRegEvent is an event that request the register file to write
// data into the register storage
type WriteRegEvent struct {
	*core.EventBase

	Req *WriteRegReq
}

// NewWriteRegEvent returns a newly created WriteRegEvent
func NewWriteRegEvent(
	time core.VTimeInSec,
	handler core.Handler,
	req *WriteRegReq,
) *WriteRegEvent {
	e := new(WriteRegEvent)
	e.EventBase = core.NewEventBase(time, handler)
	e.Req = req
	return e
}

// A RegCtrl is a Yaotsu component that is responsible for the
// timing of reading and writing registers.
//
//     <=> ToOutside the only port that the RegCtrl use to connect to the
//         outside world.
type RegCtrl struct {
	*core.ComponentBase

	Engine core.Engine

	latency core.VTimeInSec
	storage *mem.Storage
}

// NewRegCtrl returns a newly created RegCtrl
func NewRegCtrl(name string, storage *mem.Storage, engine core.Engine) *RegCtrl {
	c := new(RegCtrl)

	c.ComponentBase = core.NewComponentBase(name)
	c.storage = storage
	c.Engine = engine

	c.AddPort("ToOutside")
	return c
}

// Recv processes incomming requests, including ReadRegReq and WriteRegReq
func (c *RegCtrl) Recv(req core.Req) *core.Error {
	c.Lock()
	defer c.Unlock()

	switch req := req.(type) {
	case *ReadRegReq:
		c.processReadRegReq(req)
	case *WriteRegReq:
		c.processWriteRegReq(req)
	default:
		log.Panicf("cannor process the request of type %s", reflect.TypeOf(req))
	}
	return nil
}

func (c *RegCtrl) processReadRegReq(req *ReadRegReq) {
	evt := NewReadRegEvent(req.RecvTime()+c.latency, c, req)
	c.Engine.Schedule(evt)
}

func (c *RegCtrl) processWriteRegReq(req *WriteRegReq) {
	evt := NewWriteRegEvent(req.RecvTime()+c.latency, c, req)
	c.Engine.Schedule(evt)
}

// Handle processes the event that is scheduled on the RegCtrl
func (c *RegCtrl) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *ReadRegEvent:
		return c.handleReadRegEvent(evt)
	case *WriteRegEvent:
		return c.handleWriteRegEvent(evt)
	default:
		log.Panicf("cannot handle event event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (c *RegCtrl) handleReadRegEvent(evt *ReadRegEvent) error {
	req := evt.Req
	offset := c.getRegOffset(req.Reg) + req.Offset

	data, err := c.storage.Read(uint64(offset), uint64(req.ByteSize))
	if err != nil {
		return err
	}

	req.Buf = data
	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	c.GetConnection("ToOutside").Send(req)

	return nil
}

func (c *RegCtrl) handleWriteRegEvent(evt *WriteRegEvent) error {
	req := evt.Req
	offset := c.getRegOffset(req.Reg) + req.Offset

	err := c.storage.Write(uint64(offset), req.Buf)
	if err != nil {
		return err
	}

	req.SwapSrcAndDst()
	req.SetSendTime(evt.Time())
	c.GetConnection("ToOutside").Send(req)

	return nil
}

func (c *RegCtrl) getRegOffset(reg *insts.Reg) int {
	if reg.IsSReg() {
		return reg.RegIndex() * 4
	}

	if reg.IsVReg() {
		return reg.RegIndex() * 4
	}

	return 0
}
