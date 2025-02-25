// Package rdma provides the implementation of an RDMA engine.
package rdma

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type transaction struct {
	fromInside  sim.Msg
	fromOutside sim.Msg
	toInside    sim.Msg
	toOutside   sim.Msg
}

// An Comp is a component that helps one GPU to access the memory on
// another GPU
type Comp struct {
	*sim.TickingComponent

	ToOutside sim.Port

	ToL1 sim.Port
	ToL2 sim.Port

	CtrlPort sim.Port

	isDraining              bool
	pauseIncomingReqsFromL1 bool
	currentDrainReq         *DrainReq

	localModules           mem.AddressToPortMapper
	RemoteRDMAAddressTable mem.AddressToPortMapper

	transactionsFromOutside []transaction
	transactionsFromInside  []transaction
}

// SetLocalModuleFinder sets the table to lookup for local data.
func (c *Comp) SetLocalModuleFinder(lmf mem.AddressToPortMapper) {
	c.localModules = lmf
}

// Tick checks if make progress
func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.processFromCtrlPort() || madeProgress
	if c.isDraining {
		madeProgress = c.drainRDMA() || madeProgress
	}
	madeProgress = c.processFromL1() || madeProgress
	madeProgress = c.processFromL2() || madeProgress
	madeProgress = c.processFromOutside() || madeProgress

	return madeProgress
}

func (c *Comp) processFromCtrlPort() bool {
	req := c.CtrlPort.PeekIncoming()
	if req == nil {
		return false
	}

	req = c.CtrlPort.RetrieveIncoming()
	switch req := req.(type) {
	case *DrainReq:
		c.currentDrainReq = req
		c.isDraining = true
		c.pauseIncomingReqsFromL1 = true
		return true
	case *RestartReq:
		return c.processRDMARestartReq()
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (c *Comp) processRDMARestartReq() bool {
	restartCompleteRsp := RestartRspBuilder{}.
		WithSrc(c.CtrlPort.AsRemote()).
		WithDst(c.currentDrainReq.Src).
		Build()
	err := c.CtrlPort.Send(restartCompleteRsp)

	if err != nil {
		return false
	}
	c.currentDrainReq = nil
	c.pauseIncomingReqsFromL1 = false

	return true
}

func (c *Comp) drainRDMA() bool {
	if c.fullyDrained() {
		drainCompleteRsp := DrainRspBuilder{}.
			WithSrc(c.CtrlPort.AsRemote()).
			WithDst(c.currentDrainReq.Src).
			Build()

		err := c.CtrlPort.Send(drainCompleteRsp)
		if err != nil {
			return false
		}
		c.isDraining = false
		return true
	}
	return false
}

func (c *Comp) fullyDrained() bool {
	return len(c.transactionsFromOutside) == 0 &&
		len(c.transactionsFromInside) == 0
}

func (c *Comp) processFromL1() bool {
	if c.pauseIncomingReqsFromL1 {
		return false
	}

	madeProgress := false
	for {
		req := c.ToL1.PeekIncoming()
		if req == nil {
			return madeProgress
		}

		switch req := req.(type) {
		case mem.AccessReq:
			ret := c.processReqFromL1(req)
			if !ret {
				return madeProgress
			}

			madeProgress = true
		default:
			log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
			return false
		}
	}
}

func (c *Comp) processFromL2() bool {
	madeProgress := false
	for {
		req := c.ToL2.PeekIncoming()
		if req == nil {
			return madeProgress
		}
		switch req := req.(type) {
		case mem.AccessRsp:
			ret := c.processRspFromL2(req)
			if !ret {
				return madeProgress
			}
			madeProgress = true
		default:
			panic("unknown req type")
		}
	}
}

func (c *Comp) processFromOutside() bool {
	madeProgress := false
	for {
		req := c.ToOutside.PeekIncoming()
		if req == nil {
			return madeProgress
		}
		switch req := req.(type) {
		case mem.AccessReq:
			ret := c.processReqFromOutside(req)
			if !ret {
				return madeProgress
			}
			madeProgress = true
		case mem.AccessRsp:
			ret := c.processRspFromOutside(req)
			if !ret {
				return madeProgress
			}
			madeProgress = true
		default:
			log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
			return false
		}
	}
}

func (c *Comp) processReqFromL1(
	req mem.AccessReq,
) bool {
	dst := c.RemoteRDMAAddressTable.Find(req.GetAddress())

	// if dst == c.ToOutside.AsRemote() {
	// 	panic("RDMA loop back detected")
	// }

	cloned := c.cloneReq(req)
	cloned.Meta().Src = c.ToOutside.AsRemote()
	cloned.Meta().Dst = dst

	err := c.ToOutside.Send(cloned)
	if err == nil {
		c.ToL1.RetrieveIncoming()

		c.traceInsideOutStart(req, cloned)

		//fmt.Printf("%s req inside %s -> outside %s\n",
		//e.Name(), req.GetID(), cloned.GetID())

		trans := transaction{
			fromInside: req,
			toOutside:  cloned,
		}
		c.transactionsFromInside = append(c.transactionsFromInside, trans)

		return true
	}

	return false
}

func (c *Comp) processReqFromOutside(
	req mem.AccessReq,
) bool {
	dst := c.localModules.Find(req.GetAddress())

	cloned := c.cloneReq(req)
	cloned.Meta().Src = c.ToL2.AsRemote()
	cloned.Meta().Dst = dst

	err := c.ToL2.Send(cloned)
	if err == nil {
		c.ToOutside.RetrieveIncoming()

		c.traceOutsideInStart(req, cloned)

		//fmt.Printf("%s req outside %s -> inside %s\n",
		//e.Name(), req.GetID(), cloned.GetID())

		trans := transaction{
			fromOutside: req,
			toInside:    cloned,
		}
		c.transactionsFromOutside =
			append(c.transactionsFromOutside, trans)
		return true
	}
	return false
}

func (c *Comp) processRspFromL2(
	rsp mem.AccessRsp,
) bool {
	transactionIndex := c.findTransactionByRspToID(
		rsp.GetRspTo(), c.transactionsFromOutside)
	trans := c.transactionsFromOutside[transactionIndex]

	rspToOutside := c.cloneRsp(rsp, trans.fromOutside.Meta().ID)
	rspToOutside.Meta().Src = c.ToOutside.AsRemote()
	rspToOutside.Meta().Dst = trans.fromOutside.Meta().Src

	err := c.ToOutside.Send(rspToOutside)
	if err == nil {
		c.ToL2.RetrieveIncoming()

		//fmt.Printf("%s rsp inside %s -> outside %s\n",
		//e.Name(), rsp.GetID(), rspToOutside.GetID())

		c.traceOutsideInEnd(trans)

		c.transactionsFromOutside =
			append(c.transactionsFromOutside[:transactionIndex],
				c.transactionsFromOutside[transactionIndex+1:]...)
		return true
	}
	return false
}

func (c *Comp) processRspFromOutside(
	rsp mem.AccessRsp,
) bool {
	transactionIndex := c.findTransactionByRspToID(
		rsp.GetRspTo(), c.transactionsFromInside)
	trans := c.transactionsFromInside[transactionIndex]

	rspToInside := c.cloneRsp(rsp, trans.fromInside.Meta().ID)
	rspToInside.Meta().Src = c.ToL1.AsRemote()
	rspToInside.Meta().Dst = trans.fromInside.Meta().Src

	err := c.ToL1.Send(rspToInside)
	if err == nil {
		c.ToOutside.RetrieveIncoming()

		c.traceInsideOutEnd(trans)

		//fmt.Printf("%s rsp outside %s -> inside %s\n",
		//e.Name(), rsp.GetID(), rspToInside.GetID())

		c.transactionsFromInside =
			append(c.transactionsFromInside[:transactionIndex],
				c.transactionsFromInside[transactionIndex+1:]...)

		return true
	}

	return false
}

func (c *Comp) findTransactionByRspToID(
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

func (c *Comp) cloneReq(origin mem.AccessReq) mem.AccessReq {
	switch origin := origin.(type) {
	case *mem.ReadReq:
		read := mem.ReadReqBuilder{}.
			WithSrc(origin.Src).
			WithDst(origin.Dst).
			WithAddress(origin.Address).
			WithByteSize(origin.AccessByteSize).
			Build()
		return read
	case *mem.WriteReq:
		write := mem.WriteReqBuilder{}.
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

func (c *Comp) cloneRsp(origin mem.AccessRsp, rspTo string) mem.AccessRsp {
	switch origin := origin.(type) {
	case *mem.DataReadyRsp:
		rsp := mem.DataReadyRspBuilder{}.
			WithSrc(origin.Src).
			WithDst(origin.Dst).
			WithRspTo(rspTo).
			WithData(origin.Data).
			Build()
		return rsp
	case *mem.WriteDoneRsp:
		rsp := mem.WriteDoneRspBuilder{}.
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
func (c *Comp) SetFreq(freq sim.Freq) {
	c.TickingComponent.Freq = freq
}

func (c *Comp) traceInsideOutStart(req mem.AccessReq, cloned mem.AccessReq) {
	if len(c.Hooks()) == 0 {
		return
	}

	tracing.StartTaskWithSpecificLocation(
		tracing.MsgIDAtReceiver(req, c),
		req.Meta().ID+"_req_out",
		c,
		"req_in",
		reflect.TypeOf(req).String(),
		c.Name()+".InsideOut",
		req,
	)

	tracing.StartTaskWithSpecificLocation(
		cloned.Meta().ID+"_req_out",
		tracing.MsgIDAtReceiver(req, c),
		c,
		"req_out",
		reflect.TypeOf(req).String(),
		c.Name()+".InsideOut",
		cloned,
	)
}

func (c *Comp) traceOutsideInStart(req mem.AccessReq, cloned mem.AccessReq) {
	if len(c.Hooks()) == 0 {
		return
	}

	tracing.StartTaskWithSpecificLocation(
		tracing.MsgIDAtReceiver(req, c),
		req.Meta().ID+"_req_out",
		c,
		"req_in",
		reflect.TypeOf(req).String(),
		c.Name()+".OutsideIn",
		req,
	)

	tracing.StartTaskWithSpecificLocation(
		cloned.Meta().ID+"_req_out",
		tracing.MsgIDAtReceiver(req, c),
		c,
		"req_out",
		reflect.TypeOf(req).String(),
		c.Name()+".OutsideIn",
		cloned,
	)
}

func (c *Comp) traceInsideOutEnd(trans transaction) {
	if len(c.Hooks()) == 0 {
		return
	}

	tracing.TraceReqFinalize(trans.toOutside, c)
	tracing.TraceReqComplete(trans.fromInside, c)
}

func (c *Comp) traceOutsideInEnd(trans transaction) {
	tracing.TraceReqFinalize(trans.toInside, c)
	tracing.TraceReqComplete(trans.fromOutside, c)
}
