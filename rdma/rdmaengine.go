package rdma

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

type transaction struct {
	fromInside  akita.Req
	fromOutside akita.Req
	toInside    akita.Req
	toOutside   akita.Req
}

// An Engine is a component that helps one GPU to access the memory on
// another GPU
type Engine struct {
	*akita.ComponentBase
	ticker *akita.Ticker

	ToOutside akita.Port
	ToInside  akita.Port

	engine                 akita.Engine
	localModules           cache.LowModuleFinder
	RemoteRDMAAddressTable cache.LowModuleFinder

	transactionsFromOutside []transaction
	transactionsFromInside  []transaction

	freq     akita.Freq
	needTick bool
}

func (e *Engine) Handle(evt akita.Event) error {
	switch evt := evt.(type) {
	case akita.TickEvent:
		e.tick(evt.Time())
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (e *Engine) tick(now akita.VTimeInSec) {
	e.needTick = false

	e.processFromInside(now)
	e.processFromOutside(now)

	if e.needTick {
		e.ticker.TickLater(now)
	}
}

func (e *Engine) processFromInside(now akita.VTimeInSec) {
	req := e.ToInside.Peek()
	if req == nil {
		return
	}

	switch req := req.(type) {
	case mem.AccessReq:
		e.processReqFromInside(now, req)
	case mem.MemRsp:
		e.processRspFromInside(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *Engine) processFromOutside(now akita.VTimeInSec) {
	req := e.ToOutside.Peek()
	if req == nil {
		return
	}

	switch req := req.(type) {
	case mem.AccessReq:
		e.processReqFromOutside(now, req)
	case mem.MemRsp:
		e.processRspFromOutside(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *Engine) processReqFromInside(now akita.VTimeInSec, req mem.AccessReq) {
	dst := e.RemoteRDMAAddressTable.Find(req.GetAddress())

	cloned := e.cloneReq(req)
	cloned.SetSrc(e.ToOutside)
	cloned.SetDst(dst)
	cloned.SetSendTime(now)

	err := e.ToOutside.Send(cloned)
	if err == nil {
		e.ToInside.Retrieve(now)

		fmt.Printf("%s req inside %s -> outside %s\n",
			e.Name(), req.GetID(), cloned.GetID())

		transaction := transaction{
			fromInside: req,
			toOutside:  cloned,
		}
		e.transactionsFromInside = append(e.transactionsFromInside, transaction)
		e.needTick = true
	}
}

func (e *Engine) processReqFromOutside(now akita.VTimeInSec, req mem.AccessReq) {
	dst := e.localModules.Find(req.GetAddress())

	cloned := e.cloneReq(req)
	cloned.SetSrc(e.ToInside)
	cloned.SetDst(dst)
	cloned.SetSendTime(now)

	err := e.ToInside.Send(cloned)
	if err == nil {
		e.ToOutside.Retrieve(now)

		fmt.Printf("%s req outside %s -> inside %s\n",
			e.Name(), req.GetID(), cloned.GetID())

		transaction := transaction{
			fromOutside: req,
			toInside:    cloned,
		}
		e.transactionsFromOutside =
			append(e.transactionsFromOutside, transaction)
		e.needTick = true
	}
}

func (e *Engine) processRspFromInside(now akita.VTimeInSec, rsp mem.MemRsp) {
	transactionIndex := e.findTransactionByRspToID(
		rsp.GetRespondTo(), e.transactionsFromOutside)
	transaction := e.transactionsFromOutside[transactionIndex]

	rspToOutside := e.cloneRsp(rsp, transaction.fromOutside.GetID())
	rspToOutside.SetSendTime(now)
	rspToOutside.SetSrc(e.ToOutside)
	rspToOutside.SetDst(transaction.fromOutside.Src())

	err := e.ToOutside.Send(rspToOutside)
	if err == nil {
		e.ToInside.Retrieve(now)

		fmt.Printf("%s rsp inside %s -> outside %s\n",
			e.Name(), rsp.GetID(), rspToOutside.GetID())

		e.transactionsFromOutside =
			append(e.transactionsFromOutside[:transactionIndex],
				e.transactionsFromOutside[transactionIndex+1:]...)
		e.needTick = true
	}
}

func (e *Engine) processRspFromOutside(now akita.VTimeInSec, rsp mem.MemRsp) {
	transactionIndex := e.findTransactionByRspToID(
		rsp.GetRespondTo(), e.transactionsFromInside)
	transaction := e.transactionsFromInside[transactionIndex]

	rspToInside := e.cloneRsp(rsp, transaction.fromInside.GetID())
	rspToInside.SetSendTime(now)
	rspToInside.SetSrc(e.ToInside)
	rspToInside.SetDst(transaction.fromInside.Src())

	err := e.ToInside.Send(rspToInside)
	if err == nil {
		e.ToOutside.Retrieve(now)

		fmt.Printf("%s rsp outside %s -> inside %s\n",
			e.Name(), rsp.GetID(), rspToInside.GetID())

		e.transactionsFromInside =
			append(e.transactionsFromInside[:transactionIndex],
				e.transactionsFromInside[transactionIndex+1:]...)
		e.needTick = true
	}
}

func (e *Engine) findTransactionByRspToID(
	rspTo string,
	transactions []transaction,
) int {
	for i, trans := range transactions {
		if trans.toOutside != nil && trans.toOutside.GetID() == rspTo {
			return i
		}

		if trans.toInside != nil && trans.toInside.GetID() == rspTo {
			return i
		}
	}

	log.Panicf("transaction %s not found", rspTo)
	return 0
}

func (e *Engine) cloneReq(origin mem.AccessReq) mem.AccessReq {
	switch origin := origin.(type) {
	case *mem.ReadReq:
		read := mem.NewReadReq(origin.SendTime(),
			origin.Src(), origin.Dst(),
			origin.Address, origin.MemByteSize)
		return read
	case *mem.WriteReq:
		write := mem.NewWriteReq(origin.SendTime(),
			origin.Src(), origin.Dst(),
			origin.Address)
		write.Data = origin.Data
		return write
	default:
		log.Panicf("cannot clone request of type %s",
			reflect.TypeOf(origin))
	}
	return nil
}

func (e *Engine) cloneRsp(origin mem.MemRsp, rspTo string) mem.MemRsp {
	switch origin := origin.(type) {
	case *mem.DataReadyRsp:
		rsp := mem.NewDataReadyRsp(origin.SendTime(),
			origin.Src(), origin.Dst(), rspTo)
		rsp.Data = origin.Data
		return rsp
	case *mem.DoneRsp:
		rsp := mem.NewDoneRsp(origin.SendTime(),
			origin.Src(), origin.Dst(), rspTo)
		return rsp
	default:
		log.Panicf("cannot clone request of type %s",
			reflect.TypeOf(origin))
	}
	return nil
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
	e.RemoteRDMAAddressTable = remoteModules

	e.ToInside = akita.NewLimitNumReqPort(e, 1)
	e.ToOutside = akita.NewLimitNumReqPort(e, 1)

	return e
}
