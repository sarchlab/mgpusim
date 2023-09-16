// Package rdma provides the implementation of an RDMA engine.
package rdma

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

type transaction struct {
	fromInside  sim.Msg
	fromOutside sim.Msg
	toInside    sim.Msg
	toOutside   sim.Msg
}

// An Engine is a component that helps one GPU to access the memory on
// another GPU
type Engine struct {
	*sim.TickingComponent

	ToOutside sim.Port

	ToL1 sim.Port
	ToL2 sim.Port

	CtrlPort sim.Port

	isDraining              bool
	pauseIncomingReqsFromL1 bool
	currentDrainReq         *DrainReq

	localModules           mem.LowModuleFinder
	RemoteRDMAAddressTable mem.LowModuleFinder

	transactionsFromOutside []transaction
	transactionsFromInside  []transaction
}

// SetLocalModuleFinder sets the table to lookup for local data.
func (e *Engine) SetLocalModuleFinder(lmf mem.LowModuleFinder) {
	e.localModules = lmf
}

// Tick checks if make progress
func (e *Engine) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = e.processFromCtrlPort(now) || madeProgress
	if e.isDraining {
		madeProgress = e.drainRDMA(now) || madeProgress
	}
	madeProgress = e.processFromL1(now) || madeProgress
	madeProgress = e.processFromL2(now) || madeProgress
	madeProgress = e.processFromOutside(now) || madeProgress

	return madeProgress
}

func (e *Engine) processFromCtrlPort(now sim.VTimeInSec) bool {
	req := e.CtrlPort.Peek()
	if req == nil {
		return false
	}

	req = e.CtrlPort.Retrieve(now)
	switch req := req.(type) {
	case *DrainReq:
		e.currentDrainReq = req
		e.isDraining = true
		e.pauseIncomingReqsFromL1 = true
		return true
	case *RestartReq:
		return e.processRDMARestartReq(now)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *Engine) processRDMARestartReq(now sim.VTimeInSec) bool {
	restartCompleteRsp := RestartRspBuilder{}.
		WithSendTime(now).
		WithSrc(e.CtrlPort).
		WithDst(e.currentDrainReq.Src).
		Build()
	err := e.CtrlPort.Send(restartCompleteRsp)

	if err != nil {
		return false
	}
	e.currentDrainReq = nil
	e.pauseIncomingReqsFromL1 = false

	return true
}

func (e *Engine) drainRDMA(now sim.VTimeInSec) bool {
	if e.fullyDrained() {
		drainCompleteRsp := DrainRspBuilder{}.
			WithSendTime(now).
			WithSrc(e.CtrlPort).
			WithDst(e.currentDrainReq.Src).
			Build()

		err := e.CtrlPort.Send(drainCompleteRsp)
		if err != nil {
			return false
		}
		e.isDraining = false
		return true
	}
	return false
}

func (e *Engine) fullyDrained() bool {
	return len(e.transactionsFromOutside) == 0 &&
		len(e.transactionsFromInside) == 0
}

func (e *Engine) processFromL1(now sim.VTimeInSec) bool {
	if e.pauseIncomingReqsFromL1 {
		return false
	}

	req := e.ToL1.Peek()
	if req == nil {
		return false
	}
	switch req := req.(type) {
	case mem.AccessReq:
		flag0 := e.processReqFromL1(now, req)
		if !flag0 {
			return false
		}
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}

	for {
		req := e.ToL1.Peek()
		if req == nil {
			return true
		}
		switch req := req.(type) {
		case mem.AccessReq:
			flag1 := e.processReqFromL1(now, req)
			if !flag1 {
				return true
			}
		default:
			log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
			return false
		}
	}
}

func (e *Engine) processFromL2(now sim.VTimeInSec) bool {
	req := e.ToL2.Peek()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case mem.AccessRsp:
		flag := e.processRspFromL2(now, req)
		if !flag {
			return false
		}
	default:
		panic("unknown req type")
	}

	for {
		req := e.ToL2.Peek()
		if req == nil {
			return true
		}
		switch req := req.(type) {
		case mem.AccessRsp:
			flag := e.processRspFromL2(now, req)
			if !flag {
				return true
			}
		default:
			panic("unknown req type")
		}
	}
}

func (e *Engine) processFromOutside(now sim.VTimeInSec) bool {
	req := e.ToOutside.Peek()
	if req == nil {
		return false
	}
	switch req := req.(type) {
	case mem.AccessReq:
		flag := e.processReqFromOutside(now, req)
		if !flag {
			return false
		}
	case mem.AccessRsp:
		flag := e.processRspFromOutside(now, req)
		if !flag {
			return false
		}
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}

	for {
		req := e.ToOutside.Peek()
		if req == nil {
			return true
		}
		switch req := req.(type) {
		case mem.AccessReq:
			flag := e.processReqFromOutside(now, req)
			if !flag {
				return true
			}
		case mem.AccessRsp:
			flag := e.processRspFromOutside(now, req)
			if !flag {
				return true
			}
		default:
			log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
			return false
		}
	}
}

func (e *Engine) processReqFromL1(
	now sim.VTimeInSec,
	req mem.AccessReq,
) bool {
	dst := e.RemoteRDMAAddressTable.Find(req.GetAddress())

	if dst == e.ToOutside {
		panic("RDMA loop back detected")
	}

	cloned := e.cloneReq(req)
	cloned.Meta().Src = e.ToOutside
	cloned.Meta().Dst = dst
	cloned.Meta().SendTime = now

	err := e.ToOutside.Send(cloned)
	if err == nil {
		e.ToL1.Retrieve(now)

		tracing.TraceReqReceive(req, e)
		tracing.TraceReqInitiate(cloned, e, tracing.MsgIDAtReceiver(req, e))

		//fmt.Printf("%s req inside %s -> outside %s\n",
		//e.Name(), req.GetID(), cloned.GetID())

		trans := transaction{
			fromInside: req,
			toOutside:  cloned,
		}
		e.transactionsFromInside = append(e.transactionsFromInside, trans)

		return true
	}

	return false
}

func (e *Engine) processReqFromOutside(
	now sim.VTimeInSec,
	req mem.AccessReq,
) bool {
	dst := e.localModules.Find(req.GetAddress())

	cloned := e.cloneReq(req)
	cloned.Meta().Src = e.ToL2
	cloned.Meta().Dst = dst
	cloned.Meta().SendTime = now

	err := e.ToL2.Send(cloned)
	if err == nil {
		e.ToOutside.Retrieve(now)

		tracing.TraceReqReceive(req, e)
		tracing.TraceReqInitiate(cloned, e, tracing.MsgIDAtReceiver(req, e))

		//fmt.Printf("%s req outside %s -> inside %s\n",
		//e.Name(), req.GetID(), cloned.GetID())

		trans := transaction{
			fromOutside: req,
			toInside:    cloned,
		}
		e.transactionsFromOutside =
			append(e.transactionsFromOutside, trans)
		return true
	}
	return false
}

func (e *Engine) processRspFromL2(
	now sim.VTimeInSec,
	rsp mem.AccessRsp,
) bool {
	transactionIndex := e.findTransactionByRspToID(
		rsp.GetRspTo(), e.transactionsFromOutside)
	trans := e.transactionsFromOutside[transactionIndex]

	rspToOutside := e.cloneRsp(rsp, trans.fromOutside.Meta().ID)
	rspToOutside.Meta().SendTime = now
	rspToOutside.Meta().Src = e.ToOutside
	rspToOutside.Meta().Dst = trans.fromOutside.Meta().Src

	err := e.ToOutside.Send(rspToOutside)
	if err == nil {
		e.ToL2.Retrieve(now)

		//fmt.Printf("%s rsp inside %s -> outside %s\n",
		//e.Name(), rsp.GetID(), rspToOutside.GetID())

		tracing.TraceReqFinalize(trans.toInside, e)
		tracing.TraceReqComplete(trans.fromOutside, e)

		e.transactionsFromOutside =
			append(e.transactionsFromOutside[:transactionIndex],
				e.transactionsFromOutside[transactionIndex+1:]...)
		return true
	}
	return false
}

func (e *Engine) processRspFromOutside(
	now sim.VTimeInSec,
	rsp mem.AccessRsp,
) bool {
	transactionIndex := e.findTransactionByRspToID(
		rsp.GetRspTo(), e.transactionsFromInside)
	trans := e.transactionsFromInside[transactionIndex]

	rspToInside := e.cloneRsp(rsp, trans.fromInside.Meta().ID)
	rspToInside.Meta().SendTime = now
	rspToInside.Meta().Src = e.ToL1
	rspToInside.Meta().Dst = trans.fromInside.Meta().Src

	err := e.ToL1.Send(rspToInside)
	if err == nil {
		e.ToOutside.Retrieve(now)

		tracing.TraceReqFinalize(trans.toOutside, e)
		tracing.TraceReqComplete(trans.fromInside, e)

		//fmt.Printf("%s rsp outside %s -> inside %s\n",
		//e.Name(), rsp.GetID(), rspToInside.GetID())

		e.transactionsFromInside =
			append(e.transactionsFromInside[:transactionIndex],
				e.transactionsFromInside[transactionIndex+1:]...)

		return true
	}

	return false
}

func (e *Engine) findTransactionByRspToID(
	rspTo string,
	transactions []transaction,
) int {
	for i, trans := range transactions {
		if trans.toOutside != nil && trans.toOutside.Meta().ID == rspTo {
			return i
		}

		if trans.toInside != nil && trans.toInside.Meta().ID == rspTo {
			return i
		}
	}

	log.Panicf("transaction %s not found", rspTo)
	return 0
}

func (e *Engine) cloneReq(origin mem.AccessReq) mem.AccessReq {
	switch origin := origin.(type) {
	case *mem.ReadReq:
		read := mem.ReadReqBuilder{}.
			WithSendTime(origin.SendTime).
			WithSrc(origin.Src).
			WithDst(origin.Dst).
			WithAddress(origin.Address).
			WithByteSize(origin.AccessByteSize).
			Build()
		return read
	case *mem.WriteReq:
		write := mem.WriteReqBuilder{}.
			WithSendTime(origin.SendTime).
			WithSrc(origin.Src).
			WithDst(origin.Dst).
			WithAddress(origin.Address).
			WithData(origin.Data).
			WithDirtyMask(origin.DirtyMask).
			Build()
		return write
	default:
		log.Panicf("cannot clone request of type %s",
			reflect.TypeOf(origin))
	}
	return nil
}

func (e *Engine) cloneRsp(origin mem.AccessRsp, rspTo string) mem.AccessRsp {
	switch origin := origin.(type) {
	case *mem.DataReadyRsp:
		rsp := mem.DataReadyRspBuilder{}.
			WithSendTime(origin.SendTime).
			WithSrc(origin.Src).
			WithDst(origin.Dst).
			WithRspTo(rspTo).
			WithData(origin.Data).
			Build()
		return rsp
	case *mem.WriteDoneRsp:
		rsp := mem.WriteDoneRspBuilder{}.
			WithSendTime(origin.SendTime).
			WithSrc(origin.Src).
			WithDst(origin.Dst).
			WithRspTo(rspTo).
			Build()
		return rsp
	default:
		log.Panicf("cannot clone request of type %s",
			reflect.TypeOf(origin))
	}
	return nil
}

// SetFreq sets freq
func (e *Engine) SetFreq(freq sim.Freq) {
	e.TickingComponent.Freq = freq
}

// NewEngine creates new engine
func NewEngine(
	name string,
	engine sim.Engine,
	localModules mem.LowModuleFinder,
	remoteModules mem.LowModuleFinder,
) *Engine {
	e := new(Engine)
	e.TickingComponent = sim.NewTickingComponent(name, engine, 1*sim.GHz, e)
	e.localModules = localModules
	e.RemoteRDMAAddressTable = remoteModules

	e.ToL1 = sim.NewLimitNumMsgPort(e, 64, name+".ToL1")
	e.ToL2 = sim.NewLimitNumMsgPort(e, 64, name+".ToL2")
	e.CtrlPort = sim.NewLimitNumMsgPort(e, 64, name+".CtrlPort")
	e.ToOutside = sim.NewLimitNumMsgPort(e, 64, name+".ToOutside")

	return e
}
