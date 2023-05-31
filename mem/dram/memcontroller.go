package dram

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/addressmapping"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/cmdq"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/org"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/trans"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

// Protocol defines the category of the memory controller.
type Protocol int

// A list of all supported DRAM protocols.
const (
	DDR3 Protocol = iota
	DDR4
	GDDR5
	GDDR5X
	GDDR6
	LPDDR
	LPDDR3
	LPDDR4
	HBM
	HBM2
	HMC
)

func (p Protocol) isGDDR() bool {
	return p == GDDR5 || p == GDDR5X || p == GDDR6
}

func (p Protocol) isHBM() bool {
	return p == HBM || p == HBM2
}

// A MemController handles read and write requests.
type MemController struct {
	*sim.TickingComponent

	topPort sim.Port

	storage             *mem.Storage
	addrConverter       mem.AddressConverter
	subTransSplitter    trans.SubTransSplitter
	addrMapper          addressmapping.Mapper
	subTransactionQueue trans.SubTransactionQueue
	cmdQueue            cmdq.CommandQueue
	channel             org.Channel

	inflightTransactions []*signal.Transaction
}

// Tick updates memory controller's internal state.
func (c *MemController) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = c.respond(now) || madeProgress
	madeProgress = c.respond(now) || madeProgress
	madeProgress = c.channel.Tick(now) || madeProgress
	madeProgress = c.issue(now) || madeProgress
	madeProgress = c.subTransactionQueue.Tick(now) || madeProgress
	madeProgress = c.parseTop(now) || madeProgress
	return madeProgress
}

func (c *MemController) parseTop(now sim.VTimeInSec) (madeProgress bool) {
	msg := c.topPort.Peek()
	if msg == nil {
		return false
	}

	trans := &signal.Transaction{}
	switch msg := msg.(type) {
	case *mem.ReadReq:
		trans.Read = msg
	case *mem.WriteReq:
		trans.Write = msg
	}

	c.assignTransInternalAddress(trans)
	c.subTransSplitter.Split(trans)

	if !c.subTransactionQueue.CanPush(len(trans.SubTransactions)) {
		return false
	}

	c.subTransactionQueue.Push(trans)
	c.inflightTransactions = append(c.inflightTransactions, trans)
	c.topPort.Retrieve(now)

	tracing.TraceReqReceive(msg, c)
	for _, st := range trans.SubTransactions {
		tracing.StartTaskWithSpecificLocation(
			st.ID,
			tracing.MsgIDAtReceiver(msg, c),
			c,
			"sub-trans",
			"sub-trans",
			c.Name()+".SubTransQueue",
			nil,
		)
	}

	// fmt.Printf("%.10f, %s, start transaction, %s, %x\n",
	// 	now, c.Name(), msg.Meta().ID, trans.InternalAddress)

	return true
}

func (c *MemController) assignTransInternalAddress(trans *signal.Transaction) {
	if c.addrConverter != nil {
		trans.InternalAddress = c.addrConverter.ConvertExternalToInternal(
			trans.GlobalAddress())
		return
	}

	trans.InternalAddress = trans.GlobalAddress()
}

func (c *MemController) issue(now sim.VTimeInSec) (madeProgress bool) {
	cmd := c.cmdQueue.GetCommandToIssue(now)
	if cmd == nil {
		return false
	}

	c.channel.StartCommand(now, cmd)
	c.channel.UpdateTiming(now, cmd)

	return true
}

func (c *MemController) respond(now sim.VTimeInSec) (madeProgress bool) {
	for i, t := range c.inflightTransactions {
		if t.IsCompleted() {
			done := c.finalizeTransaction(now, t, i)
			if done {
				return true
			}
		}
	}

	return false
}

func (c *MemController) finalizeTransaction(
	now sim.VTimeInSec,
	t *signal.Transaction,
	i int,
) (done bool) {
	if t.Write != nil {
		done = c.finalizeWriteTrans(now, t, i)
		if done {
			tracing.TraceReqComplete(t.Write, c)
		}
	} else {
		done = c.finalizeReadTrans(now, t, i)
		if done {
			tracing.TraceReqComplete(t.Read, c)
		}
	}

	return done
}

func (c *MemController) finalizeWriteTrans(
	now sim.VTimeInSec,
	t *signal.Transaction,
	i int,
) (done bool) {
	err := c.storage.Write(t.InternalAddress, t.Write.Data)
	if err != nil {
		panic(err)
	}

	writeDone := mem.WriteDoneRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(t.Write.Src).
		WithRspTo(t.Write.ID).
		WithSendTime(now).
		Build()
	sendErr := c.topPort.Send(writeDone)
	if sendErr == nil {
		c.inflightTransactions = append(
			c.inflightTransactions[:i],
			c.inflightTransactions[i+1:]...)

		// fmt.Printf("%.10f, %s, finish transaction %s, %x\n",
		// 	now, c.Name(), t.Write.ID, t.InternalAddress)
		return true
	}

	return false
}

func (c *MemController) finalizeReadTrans(
	now sim.VTimeInSec,
	t *signal.Transaction,
	i int,
) (done bool) {
	data, err := c.storage.Read(t.InternalAddress, t.Read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	dataReady := mem.DataReadyRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(t.Read.Src).
		WithData(data).
		WithRspTo(t.Read.ID).
		WithSendTime(now).
		Build()
	sendErr := c.topPort.Send(dataReady)
	if sendErr == nil {
		c.inflightTransactions = append(
			c.inflightTransactions[:i],
			c.inflightTransactions[i+1:]...)

		// fmt.Printf("%.10f, %s, finish transaction %s, %x\n",
		// 	now, c.Name(), t.Read.ID, t.InternalAddress)
		return true
	}

	return false
}
