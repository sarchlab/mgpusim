package pagemigration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

var _ = Describe("Command processor middleware", func() {
	var (
		host       *testComp
		state      CPState
		mw         *CPMiddleware
		driverPort messaging.Port
		ctrlPort   messaging.Port
	)

	BeforeEach(func() {
		host = newTestHost("CP", CPDriverPort, CPCtrlPort)
		driverPort = host.GetPortByName(CPDriverPort)
		ctrlPort = host.GetPortByName(CPCtrlPort)

		state = NewCPState()
		state.AddressTranslators = []messaging.RemotePort{"AT0", "AT1"}
		state.TLBs = []messaging.RemotePort{"TLB0", "TLB1", "TLB2"}
		state.DataMover = "DataMover.Top"
		mw = NewCPMiddleware(host, &state)
	})

	request := func(msg messaging.Msg) uint64 {
		driverPort.Deliver(msg)
		mw.Tick()
		mw.Tick()

		return msg.Meta().ID
	}

	meta := func() messaging.MsgMeta {
		return messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: "Driver.PageMigration",
			Dst: driverPort.AsRemote(),
		}
	}

	// ackAll acknowledges every control or data-move request sent, and
	// returns them.
	ackAll := func() []messaging.Msg {
		msgs := drain(ctrlPort)

		for _, msg := range msgs {
			m := messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   msg.Meta().Dst,
				Dst:   msg.Meta().Src,
				RspTo: msg.Meta().ID,
			}

			switch req := msg.(type) {
			case memcontrolprotocol.Req:
				ctrlPort.Deliver(memcontrolprotocol.Rsp{
					MsgMeta: m, Command: req.Command, Success: true})
			case datamoverprotocol.DataMoveRequest:
				ctrlPort.Deliver(datamoverprotocol.DataMoveResponse{MsgMeta: m})
			}
		}

		mw.Tick()
		mw.Tick()

		return msgs
	}

	It("holds the pages in every address translator", func() {
		id := request(HoldReq{MsgMeta: meta(), PID: 1,
			VAddrs: []uint64{0x1000}})

		holds := ackAll()
		Expect(holds).To(HaveLen(2))

		for _, h := range holds {
			req := h.(memcontrolprotocol.Req)
			Expect(req.Command).To(Equal(memcontrolprotocol.CmdHold))
			Expect(req.Addresses).To(Equal([]uint64{0x1000}))
			Expect(req.PID).To(BeEquivalentTo(1))
		}

		rsps := drain(driverPort)
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Meta().RspTo).To(Equal(id))
	})

	It("answers a hold only after every translator acknowledges", func() {
		request(HoldReq{MsgMeta: meta(), PID: 1, VAddrs: []uint64{0x1000}})

		holds := drain(ctrlPort)
		ctrlPort.Deliver(memcontrolprotocol.Rsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				RspTo: holds[0].Meta().ID,
			},
			Command: memcontrolprotocol.CmdHold,
			Success: true,
		})
		mw.Tick()
		mw.Tick()

		Expect(drain(driverPort)).To(BeEmpty())
	})

	It("shoots down: pauses, invalidates, and enables every TLB", func() {
		id := request(ShootdownReq{MsgMeta: meta(), PID: 1,
			VAddrs: []uint64{0x1000}})

		for _, cmd := range []memcontrolprotocol.Command{
			memcontrolprotocol.CmdPause,
			memcontrolprotocol.CmdInvalidate,
			memcontrolprotocol.CmdEnable,
		} {
			Expect(drain(driverPort)).To(BeEmpty())

			reqs := ackAll()
			Expect(reqs).To(HaveLen(3))
			Expect(reqs[0].(memcontrolprotocol.Req).Command).To(Equal(cmd))
		}

		rsps := drain(driverPort)
		Expect(rsps).To(HaveLen(1))
		Expect(rsps[0].Meta().RspTo).To(Equal(id))
	})

	It("runs one shootdown at a time", func() {
		request(ShootdownReq{MsgMeta: meta(), PID: 1, VAddrs: []uint64{0x1000}})
		request(ShootdownReq{MsgMeta: meta(), PID: 1, VAddrs: []uint64{0x2000}})

		Expect(drain(ctrlPort)).To(HaveLen(3)) // the first one's pauses only
	})

	It("copies with the data mover, from the remote side to the local side",
		func() {
			id := request(CopyReq{MsgMeta: meta(), SrcPAddr: 0x1000,
				DstPAddr: 0x9000, Size: 4096})

			moves := ackAll()
			Expect(moves).To(HaveLen(1))
			move := moves[0].(datamoverprotocol.DataMoveRequest)
			Expect(move.Dst).To(Equal(messaging.RemotePort("DataMover.Top")))
			Expect(move.SrcSide).To(BeEquivalentTo("outside"))
			Expect(move.DstSide).To(BeEquivalentTo("inside"))
			Expect(move.ByteSize).To(Equal(uint64(4096)))

			rsps := drain(driverPort)
			Expect(rsps).To(HaveLen(1))
			Expect(rsps[0].Meta().RspTo).To(Equal(id))
			Expect(state.Ops).To(BeEmpty())
			Expect(state.Pending).To(BeEmpty())
		})
})
