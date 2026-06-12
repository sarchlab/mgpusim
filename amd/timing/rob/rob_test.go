package rob

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// noopConn is a minimal messaging.Connection used to drive the reorder
// buffer's real ports in isolation. Tests feed requests via Deliver and
// consume sent messages via RetrieveOutgoing; the port still needs a
// connection so its send/retrieve notifications have somewhere to land.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = Describe("Reorder Buffer", func() {
	const (
		topRemote        = messaging.RemotePort("Agent")
		bottomUnitRemote = messaging.RemotePort("BottomUnit")
		ctrlRemote       = messaging.RemotePort("CP")
	)

	var (
		engine     timing.Engine
		rob        *Comp
		topPort    messaging.Port
		bottomPort messaging.Port
		ctrlPort   messaging.Port

		topBufSize    int
		bottomBufSize int
	)

	makeRead := func(addr uint64) memprotocol.ReadReq {
		req := memprotocol.ReadReq{
			Address:        addr,
			AccessByteSize: 4,
		}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = topRemote
		req.Dst = topPort.AsRemote()
		req.TrafficClass = "memprotocol.ReadReq"
		return req
	}

	makeWrite := func(addr uint64, data []byte) memprotocol.WriteReq {
		req := memprotocol.WriteReq{
			Address: addr,
			Data:    data,
		}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = topRemote
		req.Dst = topPort.AsRemote()
		req.TrafficClass = "memprotocol.WriteReq"
		return req
	}

	makeWriteDone := func(rspTo uint64) memprotocol.WriteDoneRsp {
		rsp := memprotocol.WriteDoneRsp{}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = bottomUnitRemote
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = rspTo
		rsp.TrafficClass = "memprotocol.WriteDoneRsp"
		return rsp
	}

	makeDataReady := func(rspTo uint64, data []byte) memprotocol.DataReadyRsp {
		rsp := memprotocol.DataReadyRsp{Data: data}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = bottomUnitRemote
		rsp.Dst = bottomPort.AsRemote()
		rsp.RspTo = rspTo
		rsp.TrafficClass = "memprotocol.DataReadyRsp"
		return rsp
	}

	makeCtrlReq := func(cmd memcontrolprotocol.Command) memcontrolprotocol.Req {
		req := memcontrolprotocol.Req{Command: cmd}
		req.ID = timing.GetIDGenerator().Generate()
		req.Src = ctrlRemote
		req.Dst = ctrlPort.AsRemote()
		req.TrafficClass = "memcontrolprotocol.Req"
		return req
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		spec := DefaultSpec()
		spec.BufferSize = 10
		spec.NumReqPerCycle = 4
		spec.BottomUnit = bottomUnitRemote

		rob = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("ROB")

		topBufSize = 8
		bottomBufSize = 8

		assign := func(name string, bufSize int) messaging.Port {
			p := modeling.MakePortBuilder().
				WithRegistrar(reg).
				WithComponent(rob).
				WithSpec(modeling.PortSpec{BufSize: bufSize}).
				Build(name)
			rob.AssignPort(name, p)
			(&noopConn{}).PlugIn(p)
			return p
		}

		topPort = assign("Top", topBufSize)
		bottomPort = assign("Bottom", bottomBufSize)
		ctrlPort = assign("Control", 4)
	})

	Context("top-down", func() {
		It("should do nothing if buffer is full", func() {
			for i := 0; i < rob.Spec().BufferSize; i++ {
				rob.State.Transactions = append(rob.State.Transactions,
					transactionState{IsRead: true})
			}

			topPort.Deliver(makeRead(0x40))

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.State.Transactions).
				To(HaveLen(rob.Spec().BufferSize))
			Expect(topPort.PeekIncoming()).NotTo(BeNil())
		})

		It("should do nothing if no message arriving", func() {
			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should not receive request if bottom port is busy", func() {
			topPort.Deliver(makeRead(0x40))

			for i := 0; i < bottomBufSize; i++ {
				filler := memprotocol.ReadReq{Address: uint64(i)}
				filler.ID = timing.GetIDGenerator().Generate()
				filler.Src = bottomPort.AsRemote()
				filler.Dst = bottomUnitRemote
				filler.TrafficClass = "memprotocol.ReadReq"
				Expect(bottomPort.CanSend()).To(BeTrue())
				bottomPort.Send(filler)
			}

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(topPort.PeekIncoming()).NotTo(BeNil())
		})

		It("should accept request from top and forward to bottom", func() {
			read := makeRead(0x40)
			topPort.Deliver(read)

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(topPort.PeekIncoming()).To(BeNil())

			trans := rob.State.Transactions[0]
			Expect(trans.ReqFromTopID).To(Equal(read.ID))
			Expect(trans.ReqFromTopSrc).To(Equal(topRemote))
			Expect(trans.IsRead).To(BeTrue())

			sent := bottomPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(memprotocol.ReadReq{}))
			dup := sent.(memprotocol.ReadReq)
			Expect(dup.Src).To(BeIdenticalTo(bottomPort.AsRemote()))
			Expect(dup.Dst).To(BeIdenticalTo(bottomUnitRemote))
			Expect(dup.Address).To(Equal(uint64(0x40)))
			Expect(dup.AccessByteSize).To(Equal(uint64(4)))
			Expect(dup.ID).To(Equal(trans.ReqToBottomID))
			Expect(dup.ID).NotTo(Equal(read.ID))
		})
	})

	Context("parse bottom", func() {
		var writeFromTop memprotocol.WriteReq

		BeforeEach(func() {
			writeFromTop = makeWrite(0x100, []byte{1, 2, 3, 4})
			topPort.Deliver(writeFromTop)
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(1))
			bottomPort.RetrieveOutgoing()
		})

		It("should do nothing if no response in the Bottom Port", func() {
			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.State.Transactions[0].HasRsp).To(BeFalse())
		})

		It("should attach response to transaction", func() {
			rsp := makeWriteDone(rob.State.Transactions[0].ReqToBottomID)
			bottomPort.Deliver(rsp)

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.State.Transactions).To(HaveLen(1))
			Expect(rob.State.Transactions[0].HasRsp).To(BeTrue())
			Expect(bottomPort.PeekIncoming()).To(BeNil())
		})

		It("should drop a response that matches no transaction", func() {
			rsp := makeWriteDone(999999)
			bottomPort.Deliver(rsp)

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.State.Transactions[0].HasRsp).To(BeFalse())
			Expect(bottomPort.PeekIncoming()).To(BeNil())
		})
	})

	Context("bottom up", func() {
		var writeFromTop memprotocol.WriteReq

		BeforeEach(func() {
			writeFromTop = makeWrite(0x100, []byte{1, 2, 3, 4})
			topPort.Deliver(writeFromTop)
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(1))
			bottomPort.RetrieveOutgoing()

			bottomPort.Deliver(
				makeWriteDone(rob.State.Transactions[0].ReqToBottomID))
			rob.Tick()
			Expect(rob.State.Transactions[0].HasRsp).To(BeTrue())
		})

		It("should do nothing if there is no transaction", func() {
			rob.State.Transactions = nil

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
		})

		It("should do nothing if the transaction is not ready", func() {
			rob.State.Transactions[0].HasRsp = false

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.State.Transactions).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			for i := 0; i < topBufSize; i++ {
				filler := memprotocol.WriteDoneRsp{}
				filler.ID = timing.GetIDGenerator().Generate()
				filler.Src = topPort.AsRemote()
				filler.Dst = topRemote
				filler.TrafficClass = "memprotocol.WriteDoneRsp"
				Expect(topPort.CanSend()).To(BeTrue())
				topPort.Send(filler)
			}

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.State.Transactions).To(HaveLen(1))
		})

		It("should send response to top", func() {
			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.State.Transactions).To(BeEmpty())

			sent := topPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(memprotocol.WriteDoneRsp{}))
			rsp := sent.(memprotocol.WriteDoneRsp)
			Expect(rsp.Dst).To(BeIdenticalTo(topRemote))
			Expect(rsp.Src).To(BeIdenticalTo(topPort.AsRemote()))
			Expect(rsp.RspTo).To(Equal(writeFromTop.ID))
		})

		It("should return read data to top in order", func() {
			rob.State.Transactions = nil

			r1 := makeRead(0x40)
			r2 := makeRead(0x80)
			topPort.Deliver(r1)
			topPort.Deliver(r2)
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(2))
			bottomPort.RetrieveOutgoing()
			bottomPort.RetrieveOutgoing()

			// Respond out of order: the second transaction completes first.
			bottomPort.Deliver(makeDataReady(
				rob.State.Transactions[1].ReqToBottomID, []byte{0x22}))
			rob.Tick()
			rob.Tick()

			// Head is not ready, so nothing is released.
			Expect(rob.State.Transactions).To(HaveLen(2))
			Expect(topPort.RetrieveOutgoing()).To(BeNil())

			bottomPort.Deliver(makeDataReady(
				rob.State.Transactions[0].ReqToBottomID, []byte{0x11}))
			rob.Tick()
			rob.Tick()

			Expect(rob.State.Transactions).To(BeEmpty())

			out1 := topPort.RetrieveOutgoing()
			out2 := topPort.RetrieveOutgoing()
			Expect(out1.(memprotocol.DataReadyRsp).RspTo).To(Equal(r1.ID))
			Expect(out1.(memprotocol.DataReadyRsp).Data).
				To(Equal([]byte{0x11}))
			Expect(out2.(memprotocol.DataReadyRsp).RspTo).To(Equal(r2.ID))
			Expect(out2.(memprotocol.DataReadyRsp).Data).
				To(Equal([]byte{0x22}))
		})
	})

	Context("when processing control messages", func() {
		It("should pause (v4 flush)", func() {
			pause := makeCtrlReq(memcontrolprotocol.CmdPause)
			ctrlPort.Deliver(pause)

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.State.ControlState).
				To(Equal(memcontrolprotocol.StatePaused))

			ack := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
			rsp := ack.(memcontrolprotocol.Rsp)
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdPause))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(pause.ID))
		})

		It("should freeze the pipeline while paused", func() {
			rob.State.ControlState = memcontrolprotocol.StatePaused
			topPort.Deliver(makeRead(0x40))

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(topPort.PeekIncoming()).NotTo(BeNil())
		})

		It("should restart (v4 restart) via CmdReset", func() {
			topPort.Deliver(makeRead(0x40))
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(1))
			bottomPort.RetrieveOutgoing()

			rob.State.ControlState = memcontrolprotocol.StatePaused

			// Stale traffic that the restart must discard.
			topPort.Deliver(makeRead(0x80))
			bottomPort.Deliver(makeWriteDone(999999))

			reset := makeCtrlReq(memcontrolprotocol.CmdReset)
			ctrlPort.Deliver(reset)

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(rob.State.ControlState).
				To(Equal(memcontrolprotocol.StateEnabled))
			Expect(topPort.PeekIncoming()).To(BeNil())
			Expect(bottomPort.PeekIncoming()).To(BeNil())

			ack := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
			rsp := ack.(memcontrolprotocol.Rsp)
			Expect(rsp.Command).To(Equal(memcontrolprotocol.CmdReset))
			Expect(rsp.Success).To(BeTrue())
			Expect(rsp.RspTo).To(Equal(reset.ID))
		})

		It("should ack Drain only after in-flight transactions retire", func() {
			topPort.Deliver(makeWrite(0x100, []byte{1}))
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(1))
			bottomPort.RetrieveOutgoing()

			drain := makeCtrlReq(memcontrolprotocol.CmdDrain)
			ctrlPort.Deliver(drain)

			rob.Tick()
			Expect(rob.State.ControlState).
				To(Equal(memcontrolprotocol.StateDraining))
			Expect(ctrlPort.RetrieveOutgoing()).To(BeNil())

			// New top traffic is not accepted while draining.
			topPort.Deliver(makeRead(0x40))
			rob.Tick()
			Expect(rob.State.Transactions).To(HaveLen(1))

			// Let the in-flight write finish.
			bottomPort.Deliver(
				makeWriteDone(rob.State.Transactions[0].ReqToBottomID))

			var drainRsp memcontrolprotocol.Rsp
			found := false
			for i := 0; i < 8 && !found; i++ {
				rob.Tick()
				if out := ctrlPort.RetrieveOutgoing(); out != nil {
					drainRsp, found = out.(memcontrolprotocol.Rsp)
				}
			}

			Expect(found).To(BeTrue())
			Expect(drainRsp.Command).To(Equal(memcontrolprotocol.CmdDrain))
			Expect(drainRsp.Success).To(BeTrue())
			Expect(drainRsp.RspTo).To(Equal(drain.ID))
			Expect(rob.State.Transactions).To(BeEmpty())
			Expect(rob.State.ControlState).
				To(Equal(memcontrolprotocol.StatePaused))
		})

		It("should reject unsupported verbs", func() {
			flush := makeCtrlReq(memcontrolprotocol.CmdFlush)
			ctrlPort.Deliver(flush)

			madeProgress := rob.Tick()

			Expect(madeProgress).To(BeTrue())

			ack := ctrlPort.RetrieveOutgoing()
			Expect(ack).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
			rsp := ack.(memcontrolprotocol.Rsp)
			Expect(rsp.Success).To(BeFalse())
			Expect(rsp.Error).To(Equal(memcontrolprotocol.ErrUnsupported))
		})
	})
})
