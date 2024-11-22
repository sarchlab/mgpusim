// Package rdma provides the implementation of an RDMA engine.
package rdma

import (
	"fmt"
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

	localModules           mem.LowModuleFinder
	RemoteRDMAAddressTable mem.LowModuleFinder

	transactionsFromOutside []transaction
	transactionsFromInside  []transaction
}

func extractIDs(bufferElements []interface{}) []string {
    ids := []string{}
    for _, elem := range bufferElements {
        if req, ok := elem.(*mem.ReadReq); ok {
            ids = append(ids, req.MsgMeta.ID)
        }
    }
    return ids
}

// SetLocalModuleFinder sets the table to lookup for local data.
func (c *Comp) SetLocalModuleFinder(lmf mem.LowModuleFinder) {
	c.localModules = lmf
}

// Tick checks if make progress
func (c *Comp) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = c.processFromCtrlPort(now) || madeProgress
	if c.isDraining {
		madeProgress = c.drainRDMA(now) || madeProgress
	}
	madeProgress = c.processFromL1(now) || madeProgress
	madeProgress = c.processFromL2(now) || madeProgress
	madeProgress = c.processFromOutside(now) || madeProgress

	return madeProgress
}

func (c *Comp) processFromCtrlPort(now sim.VTimeInSec) bool {
	req := c.CtrlPort.Peek()
	if req == nil {
		tracing.TraceDelay(nil, c, c.CtrlPort.Name(), now, "Delay", "idle", "rdma/comp")
		fmt.Printf("Delay, no available ctrl port: %.20f\n", now)
		return false
	}

	req = c.CtrlPort.Retrieve(now)
	switch req := req.(type) {
	case *DrainReq:
		c.currentDrainReq = req
		c.isDraining = true
		c.pauseIncomingReqsFromL1 = true
		// tracing.TraceDelay(req, c, c.CtrlPort.Name(), now, "Step", "", "rdma/comp")
		transactionProgressID := sim.GetIDGenerator().Generate();
		tracing.TraceProgress(transactionProgressID, req.Meta().ID, c, now, "rdma/comp", "Port not available")
		dependentIDs := extractIDs(c.CtrlPort.GetAllBufferElements());
		tracing.TraceDependency(transactionProgressID, c, dependentIDs)
		
		return true
	case *RestartReq:
		return c.processRDMARestartReq(now)
	default:
		delayReason := fmt.Sprintf("cannot process request of type %s", reflect.TypeOf(req))
		tracing.TraceDelay(req, c, c.CtrlPort.Name(), now, "Delay", delayReason, "rdma/comp")
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (c *Comp) processRDMARestartReq(now sim.VTimeInSec) bool {
	restartCompleteRsp := RestartRspBuilder{}.
		WithSendTime(now).
		WithSrc(c.CtrlPort).
		WithDst(c.currentDrainReq.Src).
		Build()
	err := c.CtrlPort.Send(restartCompleteRsp)

	if err != nil {
		fmt.Printf("Delay, fail to send ctrl port: %.20f\n", now)
		return false
	}
	c.currentDrainReq = nil
	c.pauseIncomingReqsFromL1 = false

	// tracing.TraceDelay(restartCompleteRsp, c, c.CtrlPort.Name(), now, "Step", "", "rdma/comp")
	transactionProgressID := sim.GetIDGenerator().Generate();
	tracing.TraceProgress(transactionProgressID, restartCompleteRsp.Meta().ID, c, now, "rdma/comp", "Port network not available")
	dependentIDs := extractIDs(c.CtrlPort.GetAllBufferElements());
	tracing.TraceDependency(transactionProgressID, c, dependentIDs)

	return true
}

func (c *Comp) drainRDMA(now sim.VTimeInSec) bool {
	if c.fullyDrained() {
		drainCompleteRsp := DrainRspBuilder{}.
			WithSendTime(now).
			WithSrc(c.CtrlPort).
			WithDst(c.currentDrainReq.Src).
			Build()

		err := c.CtrlPort.Send(drainCompleteRsp)
		if err != nil {
			tracing.TraceDelay(drainCompleteRsp, c, c.CtrlPort.Name(), now, "Delay", "Port network not available", "rdma/comp")
			fmt.Printf("Delay, fail to send ctrl port: %.20f\n", now)
			return false
		}
		c.isDraining = false
		// tracing.TraceDelay(drainCompleteRsp, c, c.CtrlPort.Name(), now, "Step", "", "rdma/comp")
		transactionProgressID := sim.GetIDGenerator().Generate();
		tracing.TraceProgress(transactionProgressID, drainCompleteRsp.Meta().ID, c, now, "rdma/comp","Port network not available")
		dependentIDs := extractIDs(c.CtrlPort.GetAllBufferElements());
		tracing.TraceDependency(transactionProgressID, c, dependentIDs)
		return true
	}
	tracing.TraceDelay(nil, c, c.CtrlPort.Name(), now, "Delay", "RDMA not fully drained", "rdma/comp")
	return false
}

func (c *Comp) fullyDrained() bool {
	return len(c.transactionsFromOutside) == 0 &&
		len(c.transactionsFromInside) == 0
}

func (c *Comp) processFromL1(now sim.VTimeInSec) bool {
	if c.pauseIncomingReqsFromL1 {
		fmt.Printf("Delay, pauseIncomingReqsFromL1: %.20f\n", now)
		return false
	}

	madeProgress := false
	for {
		req := c.ToL1.Peek()
		if req == nil {
			tracing.TraceDelay(req, c, c.ToL1.Name(), now, "Delay", "data not available", "rdma/comp")
			fmt.Printf("Delay, no available ToL1 port: %.20f\n", now)
			return madeProgress
		}

		switch req := req.(type) {
		case mem.AccessReq:
			ret := c.processReqFromL1(now, req)
			if !ret {
				fmt.Printf("Delay, no available ToL1 port: %.20f\n", now)
				tracing.TraceDelay(req, c, c.ToL1.Name(), now, "Delay", "Resources/storage not available", "rdma/comp")
				return madeProgress
			}
			// tracing.TraceDelay(req, c, c.ToL1.Name(), now, "Step", "", "rdma/comp")
			transactionProgressID := sim.GetIDGenerator().Generate();
			tracing.TraceProgress(transactionProgressID, req.Meta().ID, c, now, "rdma/comp", "Resources/storage not available")
			dependentIDs := extractIDs(c.ToL1.GetAllBufferElements());
			tracing.TraceDependency(transactionProgressID, c, dependentIDs)

			madeProgress = true
		default:
			delayReason := fmt.Sprintf("cannot process request of type %s", reflect.TypeOf(req))
			tracing.TraceDelay(req, c, c.ToL1.Name(), now, "Delay", delayReason, "rdma/comp")
			fmt.Printf("Delay, no available ToL1 port: %.20f\n", now)
			return false
		}
	}
}

func (c *Comp) processFromL2(now sim.VTimeInSec) bool {
	madeProgress := false
	for {
		req := c.ToL2.Peek()
		if req == nil {
			tracing.TraceDelay(req, c, c.ToL2.Name(), now, "Delay", "data not available", "rdma/comp")
			return madeProgress
		}
		switch req := req.(type) {
		case mem.AccessRsp:
			ret := c.processRspFromL2(now, req)
			if !ret {
				tracing.TraceDelay(req, c, c.ToL2.Name(), now, "Delay", "Resources/storage not available", "rdma/comp")
				return madeProgress
			}
			// tracing.TraceDelay(req, c, c.ToL2.Name(), now, "Step", "", "rdma/comp")
			transactionProgressID := sim.GetIDGenerator().Generate();
			tracing.TraceProgress(transactionProgressID, req.Meta().ID, c, now, "rdma/comp","Resources/storage not available")
			dependentIDs := extractIDs(c.ToL2.GetAllBufferElements());
			tracing.TraceDependency(transactionProgressID, c, dependentIDs)
			madeProgress = true
		default:
			panic("unknown req type")
		}
	}
}

func (c *Comp) processFromOutside(now sim.VTimeInSec) bool {
	madeProgress := false
	for {
		req := c.ToOutside.Peek()
		if req == nil {
			tracing.TraceDelay(req, c, c.ToOutside.Name(), now, "Delay", "data not available", "rdma/comp")
			return madeProgress
		}
		switch req := req.(type) {
		case mem.AccessReq:
			ret := c.processReqFromOutside(now, req)
			if !ret {
				tracing.TraceDelay(req, c, c.ToOutside.Name(), now, "Delay", "Resources/storage not available", "rdma/comp")
				return madeProgress
			}
			// tracing.TraceDelay(req, c, c.ToOutside.Name(), now, "Step", "", "rdma/comp")
			transactionProgressID := sim.GetIDGenerator().Generate();
			tracing.TraceProgress(transactionProgressID, req.Meta().ID, c, now, "rdma/comp","Resources/storage not available")
			dependentIDs := extractIDs(c.ToOutside.GetAllBufferElements());
			tracing.TraceDependency(transactionProgressID, c, dependentIDs)
			madeProgress = true
		case mem.AccessRsp:
			ret := c.processRspFromOutside(now, req)
			if !ret {
				tracing.TraceDelay(req, c, c.ToOutside.Name(), now, "Delay", "Resources/storage not available", "rdma/comp")
				return madeProgress
			}
			// tracing.TraceDelay(req, c, c.ToOutside.Name(), now, "Step", "", "rdma/comp")
			transactionProgressID := sim.GetIDGenerator().Generate();
			tracing.TraceProgress(transactionProgressID, req.Meta().ID, c, now, "rdma/comp","Resources/storage not available")
			dependentIDs := extractIDs(c.ToOutside.GetAllBufferElements());
			tracing.TraceDependency(transactionProgressID, c, dependentIDs)
			madeProgress = true
		default:
			log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
			return false
		}
	}
}

func (c *Comp) processReqFromL1(
	now sim.VTimeInSec,
	req mem.AccessReq,
) bool {
	dst := c.RemoteRDMAAddressTable.Find(req.GetAddress())

	if dst == c.ToOutside {
		panic("RDMA loop back detected")
	}

	cloned := c.cloneReq(req)
	cloned.Meta().Src = c.ToOutside
	cloned.Meta().Dst = dst
	cloned.Meta().SendTime = now

	err := c.ToOutside.Send(cloned)
	if err == nil {
		c.ToL1.Retrieve(now)

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
	now sim.VTimeInSec,
	req mem.AccessReq,
) bool {
	dst := c.localModules.Find(req.GetAddress())

	cloned := c.cloneReq(req)
	cloned.Meta().Src = c.ToL2
	cloned.Meta().Dst = dst
	cloned.Meta().SendTime = now

	err := c.ToL2.Send(cloned)
	if err == nil {
		c.ToOutside.Retrieve(now)

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
	now sim.VTimeInSec,
	rsp mem.AccessRsp,
) bool {
	transactionIndex := c.findTransactionByRspToID(
		rsp.GetRspTo(), c.transactionsFromOutside)
	trans := c.transactionsFromOutside[transactionIndex]

	rspToOutside := c.cloneRsp(rsp, trans.fromOutside.Meta().ID)
	rspToOutside.Meta().SendTime = now
	rspToOutside.Meta().Src = c.ToOutside
	rspToOutside.Meta().Dst = trans.fromOutside.Meta().Src

	err := c.ToOutside.Send(rspToOutside)
	if err == nil {
		c.ToL2.Retrieve(now)

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
	now sim.VTimeInSec,
	rsp mem.AccessRsp,
) bool {
	transactionIndex := c.findTransactionByRspToID(
		rsp.GetRspTo(), c.transactionsFromInside)
	trans := c.transactionsFromInside[transactionIndex]

	rspToInside := c.cloneRsp(rsp, trans.fromInside.Meta().ID)
	rspToInside.Meta().SendTime = now
	rspToInside.Meta().Src = c.ToL1
	rspToInside.Meta().Dst = trans.fromInside.Meta().Src

	err := c.ToL1.Send(rspToInside)
	if err == nil {
		c.ToOutside.Retrieve(now)

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

func (c *Comp) cloneRsp(origin mem.AccessRsp, rspTo string) mem.AccessRsp {
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
