package rdma

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

// An Engine is a component that helps one GPU to access the memory on
// another GPU
type Engine struct {
	*akita.ComponentBase
	ticker *akita.Ticker

	ToOutside akita.Port
	ToInside  akita.Port

	engine        akita.Engine
	localModules  cache.LowModuleFinder
	remoteModules cache.LowModuleFinder
	originalSrc   map[string]akita.Port

	freq     akita.Freq
	needTick bool
}

func (e *Engine) Handle(evt akita.Event) error {
	switch evt := evt.(type) {
	case *akita.TickEvent:
		e.tick(evt.Time())
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (e *Engine) tick(now akita.VTimeInSec) {
	e.needTick = false

	e.processReqFromInside(now)
	e.processReqFromOutside(now)

	if e.needTick {
		e.ticker.TickLater(now)
	}
}

func (e *Engine) processReqFromInside(now akita.VTimeInSec) {
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

func (e *Engine) sendReqToOutside(now akita.VTimeInSec, req akita.Req, dst akita.Port) {
	originalSrc := req.Src()
	req.SetSrc(e.ToOutside)
	req.SetDst(dst)
	req.SetSendTime(now)
	err := e.ToOutside.Send(req)
	if err == nil {
		e.ToInside.Retrieve(now)
		e.originalSrc[req.GetID()] = originalSrc
	} else {
		req.SetSrc(originalSrc)
		req.SetDst(e.ToInside)
	}
}

func (e *Engine) sendRspToOutside(now akita.VTimeInSec, req mem.MemRsp) {
	recoverSrc := req.Src()
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
	} else {
		req.SetSrc(recoverSrc)
		req.SetDst(e.ToInside)
	}
}

func (e *Engine) processReqFromOutside(now akita.VTimeInSec) {
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

func (e *Engine) sendReqToInside(now akita.VTimeInSec, req akita.Req, dst akita.Port) {
	originalSrc := req.Src()
	req.SetSrc(e.ToInside)
	req.SetDst(dst)
	req.SetSendTime(now)
	err := e.ToInside.Send(req)
	if err == nil {
		e.ToOutside.Retrieve(now)
		e.originalSrc[req.GetID()] = originalSrc
	} else {
		req.SetSrc(originalSrc)
		req.SetDst(e.ToOutside)
	}
}

func (e *Engine) sendRspToInside(now akita.VTimeInSec, req mem.MemRsp) {
	recoverSrc := req.Src()
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
	} else {
		req.SetSrc(recoverSrc)
		req.SetDst(e.ToOutside)
	}
}

func (e *Engine) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	e.ticker.TickLater(now)
}

func (e *Engine) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
	e.ticker.TickLater(now)
}

func (e *Engine) SetFreq(freq akita.Freq) {
	e.freq = freq
}

func NewEngine(
	name string,
	engine akita.Engine,
	localModules cache.LowModuleFinder,
	remoteModules cache.LowModuleFinder,
) *Engine {
	e := new(Engine)
	e.freq = 1 * akita.GHz
	e.ComponentBase = akita.NewComponentBase(name)
	e.ticker = akita.NewTicker(e, engine, e.freq)

	e.engine = engine
	e.localModules = localModules
	e.remoteModules = remoteModules

	e.originalSrc = make(map[string]akita.Port)

	e.ToInside = akita.NewLimitNumReqPort(e, 1)
	e.ToOutside = akita.NewLimitNumReqPort(e, 1)

	return e
}
