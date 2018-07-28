package rdma

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

// An Engine is a component that helps one GPU to access the memory on
// another GPU
type Engine struct {
	*core.ComponentBase
	ticker *core.Ticker

	ToOutside *core.Port
	ToInside  *core.Port

	engine        core.Engine
	localModules  cache.LowModuleFinder
	remoteModules cache.LowModuleFinder
	originalSrc   map[string]*core.Port

	freq     core.Freq
	needTick bool
}

func (e *Engine) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *core.TickEvent:
		e.tick(evt.Time())
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (e *Engine) tick(now core.VTimeInSec) {
	e.needTick = false

	e.processReqFromInside(now)
	e.processReqFromOutside(now)

	if e.needTick {
		e.ticker.TickLater(now)
	}
}

func (e *Engine) processReqFromInside(now core.VTimeInSec) {
	req := e.ToInside.Peek()
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *mem.ReadReq:
		dst := e.remoteModules.Find(req.Address)
		e.sendReqToOutside(now, req, dst)
	case *mem.WriteReq:
		dst := e.remoteModules.Find(req.Address)
		e.sendReqToOutside(now, req, dst)
	case *mem.DataReadyRsp:
		e.sendRspToOutside(now, req)
	case *mem.DoneRsp:
		e.sendRspToOutside(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *Engine) sendReqToOutside(now core.VTimeInSec, req core.Req, dst *core.Port) {
	originalSrc := req.Src()
	req.SetSrc(e.ToOutside)
	req.SetDst(dst)
	req.SetSendTime(now)
	err := e.ToOutside.Send(req)
	if err == nil {
		e.ToInside.Retrieve(now)
		e.originalSrc[req.GetID()] = originalSrc
	}
}

func (e *Engine) sendRspToOutside(now core.VTimeInSec, req mem.MemRsp) {
	src, found := e.originalSrc[req.GetRespondTo()]
	if !found {
		log.Panic("original src not found")
	}
	req.SetDst(src)
	req.SetSrc(e.ToOutside)
	req.SetSendTime(now)
	err := e.ToOutside.Send(req)
	if err == nil {
		e.ToInside.Retrieve(now)
		delete(e.originalSrc, req.GetRespondTo())
	}
}

func (e *Engine) processReqFromOutside(now core.VTimeInSec) {
	req := e.ToOutside.Peek()
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *mem.ReadReq:
		dst := e.localModules.Find(req.Address)
		e.sendReqToInside(now, req, dst)
	case *mem.WriteReq:
		dst := e.localModules.Find(req.Address)
		e.sendReqToInside(now, req, dst)
	case *mem.DataReadyRsp:
		e.sendRspToInside(now, req)
	case *mem.DoneRsp:
		e.sendRspToInside(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *Engine) sendReqToInside(now core.VTimeInSec, req core.Req, dst *core.Port) {
	originalSrc := req.Src()
	req.SetSrc(e.ToInside)
	req.SetDst(dst)
	req.SetSendTime(now)
	err := e.ToInside.Send(req)
	if err == nil {
		e.ToOutside.Retrieve(now)
		e.originalSrc[req.GetID()] = originalSrc
	}
}

func (e *Engine) sendRspToInside(now core.VTimeInSec, req mem.MemRsp) {
	src, found := e.originalSrc[req.GetRespondTo()]
	if !found {
		log.Panic("original src not found")
	}
	req.SetDst(src)
	req.SetSrc(e.ToInside)
	req.SetSendTime(now)
	err := e.ToInside.Send(req)
	if err == nil {
		e.ToOutside.Retrieve(now)
		delete(e.originalSrc, req.GetRespondTo())
	}
}

func (e *Engine) NotifyRecv(now core.VTimeInSec, port *core.Port) {
	e.ticker.TickLater(now)
}

func (e *Engine) NotifyPortFree(now core.VTimeInSec, port *core.Port) {
	e.ticker.TickLater(now)
}

func (e *Engine) SetFreq(freq core.Freq) {
	e.freq = freq
}

func NewEngine(
	name string,
	engine core.Engine,
	localModules cache.LowModuleFinder,
	remoteModules cache.LowModuleFinder,
) *Engine {
	e := new(Engine)
	e.freq = 1 * core.GHz
	e.ComponentBase = core.NewComponentBase(name)
	e.ticker = core.NewTicker(e, engine, e.freq)

	e.engine = engine
	e.localModules = localModules
	e.remoteModules = remoteModules

	e.originalSrc = make(map[string]*core.Port)

	e.ToInside = core.NewPort(e)
	e.ToOutside = core.NewPort(e)

	return e
}
