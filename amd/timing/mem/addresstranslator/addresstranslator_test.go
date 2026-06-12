package addresstranslator

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// noopConn is a minimal messaging.Connection used to drive a component's
// ports in isolation. Tests feed requests with Deliver and read responses
// with RetrieveOutgoing; the port still needs a connection so its
// send/retrieve notifications have somewhere to go.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

const (
	topBufSize         = 4
	bottomBufSize      = 4
	translationBufSize = 4
	ctrlBufSize        = 1
)

// assignPort builds a port with the given buffer size using the same registrar
// the component was built with, and assigns it to the component's declared port
// of the same name.
func assignPort(
	reg modeling.Registrar,
	comp *Comp,
	name string,
	bufSize int,
) messaging.Port {
	p := modeling.MakePortBuilder().
		WithRegistrar(reg).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, p)
	return p
}

// assignPorts assigns the translator's four declared ports (Top, Bottom,
// Translation, Control).
func assignPorts(reg modeling.Registrar, comp *Comp, topBufSize int) {
	assignPort(reg, comp, "Top", topBufSize)
	assignPort(reg, comp, "Bottom", bottomBufSize)
	assignPort(reg, comp, "Translation", translationBufSize)
	assignPort(reg, comp, "Control", ctrlBufSize)
}

var _ = Describe("Address Translator", func() {
	var (
		engine          timing.Engine
		t               *Comp
		topPort         messaging.Port
		bottomPort      messaging.Port
		translationPort messaging.Port
		ctrlPort        messaging.Port
		tParseTransMW   *parseTranslateMW
		tRespondPipeMW  *respondPipelineMW
	)

	build := func(topBufSize int) {
		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.Freq = 1

		resources := Resources{
			MemProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("MemPort"),
			},
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("TranslationProvider"),
			},
		}

		reg := modeling.NewStandaloneRegistrar(engine)
		t = MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			WithResources(resources).
			Build("AddressTranslator")

		assignPorts(reg, t, topBufSize)

		topPort = t.GetPortByName("Top")
		bottomPort = t.GetPortByName("Bottom")
		translationPort = t.GetPortByName("Translation")
		ctrlPort = t.GetPortByName("Control")

		for _, p := range []messaging.Port{
			topPort, bottomPort, translationPort, ctrlPort,
		} {
			conn := &noopConn{}
			conn.PlugIn(p)
		}

		tParseTransMW = t.Middlewares()[1].(*parseTranslateMW)
		tRespondPipeMW = t.Middlewares()[2].(*respondPipelineMW)
	}

	BeforeEach(func() {
		engine = timing.NewSerialEngine()
		build(topBufSize)
	})

	Context("translate stage", func() {
		var (
			req memprotocol.ReadReq
		)

		BeforeEach(func() {
			req = memprotocol.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Src = messaging.RemotePort("Agent")
			req.Dst = topPort.AsRemote()
			req.Address = 0x100
			req.AccessByteSize = 4
			req.PID = 1
			req.TrafficBytes = 12
			req.TrafficClass = "memprotocol.ReadReq"
		})

		It("should do nothing if there is no request", func() {
			madeProgress := tParseTransMW.translate()
			Expect(madeProgress).To(BeFalse())
		})

		It("should send translation", func() {
			req.Address = 0x1040
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(1))
			Expect(updatedState.Transactions[0].TranslationVAddr).
				To(Equal(uint64(0x1000)))
			Expect(updatedState.Transactions[0].TranslationPID).
				To(Equal(vm.PID(1)))

			sent := translationPort.RetrieveOutgoing()
			Expect(sent).To(BeAssignableToTypeOf(vmprotocol.TranslationReq{}))
			transReq := sent.(vmprotocol.TranslationReq)
			Expect(updatedState.Transactions[0].TranslationReqID).
				To(Equal(transReq.ID))
		})

		It("should stall if cannot send for translation", func() {
			fillOutgoing(translationPort, translationBufSize)
			topPort.Deliver(req)

			needTick := tParseTransMW.translate()

			Expect(needTick).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(0))
		})

		It("should coalesce requests to the same page", func() {
			req.Address = 0x1040
			topPort.Deliver(req)

			req2 := memprotocol.ReadReq{}
			req2.ID = timing.GetIDGenerator().Generate()
			req2.Src = messaging.RemotePort("Agent")
			req2.Dst = topPort.AsRemote()
			req2.Address = 0x1080
			req2.AccessByteSize = 4
			req2.PID = 1
			req2.TrafficBytes = 12
			req2.TrafficClass = "memprotocol.ReadReq"
			topPort.Deliver(req2)

			Expect(tParseTransMW.translate()).To(BeTrue())
			Expect(tParseTransMW.translate()).To(BeTrue())

			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(1))
			Expect(updatedState.Transactions[0].IncomingReqs).To(HaveLen(2))

			// Only one translation request goes out.
			Expect(translationPort.RetrieveOutgoing()).NotTo(BeNil())
			Expect(translationPort.RetrieveOutgoing()).To(BeNil())
		})

		It("should not coalesce requests with different PIDs", func() {
			req.Address = 0x1040
			topPort.Deliver(req)

			req2 := memprotocol.ReadReq{}
			req2.ID = timing.GetIDGenerator().Generate()
			req2.Src = messaging.RemotePort("Agent")
			req2.Dst = topPort.AsRemote()
			req2.Address = 0x1080
			req2.AccessByteSize = 4
			req2.PID = 2
			req2.TrafficBytes = 12
			req2.TrafficClass = "memprotocol.ReadReq"
			topPort.Deliver(req2)

			Expect(tParseTransMW.translate()).To(BeTrue())
			Expect(tParseTransMW.translate()).To(BeTrue())

			updatedState := &t.State
			Expect(updatedState.Transactions).To(HaveLen(2))
		})
	})

	Context("parse translation", func() {
		var (
			transReq1, transReq2 vmprotocol.TranslationReq
		)

		BeforeEach(func() {
			transReq1 = vmprotocol.TranslationReq{}
			transReq1.ID = timing.GetIDGenerator().Generate()
			transReq1.PID = 1
			transReq1.VAddr = 0x100
			transReq1.DeviceID = 1
			transReq1.TrafficClass = "vmprotocol.TranslationReq"
			transReq2 = vmprotocol.TranslationReq{}
			transReq2.ID = timing.GetIDGenerator().Generate()
			transReq2.PID = 1
			transReq2.VAddr = 0x100
			transReq2.DeviceID = 1
			transReq2.TrafficClass = "vmprotocol.TranslationReq"

			t.State = State{
				Transactions: []transactionState{
					{TranslationReqID: transReq1.ID},
					{TranslationReqID: transReq2.ID},
				},
			}
		})

		It("should do nothing if there is no translation return", func() {
			needTick := tRespondPipeMW.parseTranslation()
			Expect(needTick).To(BeFalse())
		})

		It("should stall if send failed", func() {
			req := memprotocol.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Address = 0x10040
			req.AccessByteSize = 4
			req.TrafficBytes = 12
			req.TrafficClass = "memprotocol.ReadReq"
			translationRsp := vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vmprotocol.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req),
						},
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			fillOutgoing(bottomPort, bottomBufSize)
			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeFalse())
		})

		It("should forward read request", func() {
			req := memprotocol.ReadReq{}
			req.ID = timing.GetIDGenerator().Generate()
			req.Address = 0x10040
			req.AccessByteSize = 4
			req.TrafficBytes = 12
			req.TrafficClass = "memprotocol.ReadReq"
			translationRsp := vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vmprotocol.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req),
						},
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeTrue())

			sent := bottomPort.RetrieveOutgoing()
			read := sent.(memprotocol.ReadReq)
			Expect(read.PID).To(Equal(vm.PID(0)))
			Expect(read.Address).To(Equal(uint64(0x20040)))
			Expect(read.AccessByteSize).To(Equal(uint64(4)))
			Expect(read.Src).To(Equal(bottomPort.AsRemote()))

			updatedState := &t.State
			Expect(updatedState.Transactions).NotTo(
				ContainElement(
					WithTransform(
						func(ts transactionState) uint64 { return ts.TranslationReqID },
						Equal(transReq1.ID),
					),
				),
			)
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should forward write request", func() {
			data := []byte{1, 2, 3, 4}
			dirty := []bool{false, true, false, true}
			write := memprotocol.WriteReq{}
			write.ID = timing.GetIDGenerator().Generate()
			write.Address = 0x10040
			write.Data = data
			write.DirtyMask = dirty
			write.TrafficBytes = len(data) + 12
			write.TrafficClass = "memprotocol.WriteReq"
			translationRsp := vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vmprotocol.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(write),
						},
					},
					{TranslationReqID: transReq2.ID},
				},
			}

			translationPort.Deliver(translationRsp)

			madeProgress := tRespondPipeMW.parseTranslation()

			Expect(madeProgress).To(BeTrue())

			sent := bottomPort.RetrieveOutgoing()
			writeMsg := sent.(memprotocol.WriteReq)
			Expect(writeMsg.PID).To(Equal(vm.PID(0)))
			Expect(writeMsg.Address).To(Equal(uint64(0x20040)))
			Expect(writeMsg.Src).To(Equal(bottomPort.AsRemote()))
			Expect(writeMsg.Data).To(Equal(data))
			Expect(writeMsg.DirtyMask).To(Equal(dirty))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should drain coalesced requests one by one", func() {
			req1 := memprotocol.ReadReq{}
			req1.ID = timing.GetIDGenerator().Generate()
			req1.Address = 0x10040
			req1.AccessByteSize = 4
			req1.TrafficBytes = 12
			req1.TrafficClass = "memprotocol.ReadReq"
			req2 := memprotocol.ReadReq{}
			req2.ID = timing.GetIDGenerator().Generate()
			req2.Address = 0x10080
			req2.AccessByteSize = 4
			req2.TrafficBytes = 12
			req2.TrafficClass = "memprotocol.ReadReq"
			translationRsp := vmprotocol.TranslationRsp{
				Page: vm.Page{
					PID:   1,
					VAddr: 0x10000,
					PAddr: 0x20000,
				},
			}
			translationRsp.ID = timing.GetIDGenerator().Generate()
			translationRsp.RspTo = transReq1.ID
			translationRsp.TrafficClass = "vmprotocol.TranslationRsp"

			t.State = State{
				Transactions: []transactionState{
					{
						TranslationReqID: transReq1.ID,
						TranslationVAddr: 0x10000,
						TranslationPID:   1,
						IncomingReqs: []incomingReqState{
							msgToIncomingReqState(req1),
							msgToIncomingReqState(req2),
						},
					},
				},
			}

			translationPort.Deliver(translationRsp)

			// First call processes the translation response and forwards req1.
			Expect(tRespondPipeMW.parseTranslation()).To(BeTrue())
			sent1 := bottomPort.RetrieveOutgoing().(memprotocol.ReadReq)
			Expect(sent1.Address).To(Equal(uint64(0x20040)))
			Expect(t.State.Transactions).To(HaveLen(1))
			Expect(t.State.Transactions[0].TranslationDone).To(BeTrue())

			// Second call drains the coalesced req2 using the stored page.
			Expect(tRespondPipeMW.parseTranslation()).To(BeTrue())
			sent2 := bottomPort.RetrieveOutgoing().(memprotocol.ReadReq)
			Expect(sent2.Address).To(Equal(uint64(0x20080)))
			Expect(t.State.Transactions).To(BeEmpty())
			Expect(t.State.InflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("respond", func() {
		var (
			readFromTop   memprotocol.ReadReq
			writeFromTop  memprotocol.WriteReq
			readToBottom  memprotocol.ReadReq
			writeToBottom memprotocol.WriteReq
		)

		BeforeEach(func() {
			readFromTop = memprotocol.ReadReq{}
			readFromTop.ID = timing.GetIDGenerator().Generate()
			readFromTop.Src = messaging.RemotePort("Agent")
			readFromTop.Dst = topPort.AsRemote()
			readFromTop.Address = 0x10040
			readFromTop.AccessByteSize = 4
			readFromTop.TrafficBytes = 12
			readFromTop.TrafficClass = "memprotocol.ReadReq"
			readToBottom = memprotocol.ReadReq{}
			readToBottom.ID = timing.GetIDGenerator().Generate()
			readToBottom.Src = bottomPort.AsRemote()
			readToBottom.Dst = messaging.RemotePort("MemPort")
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "memprotocol.ReadReq"
			writeFromTop = memprotocol.WriteReq{}
			writeFromTop.ID = timing.GetIDGenerator().Generate()
			writeFromTop.Src = messaging.RemotePort("Agent")
			writeFromTop.Dst = topPort.AsRemote()
			writeFromTop.Address = 0x10040
			writeFromTop.TrafficBytes = 12
			writeFromTop.TrafficClass = "memprotocol.WriteReq"
			writeToBottom = memprotocol.WriteReq{}
			writeToBottom.ID = timing.GetIDGenerator().Generate()
			writeToBottom.Src = bottomPort.AsRemote()
			writeToBottom.Dst = messaging.RemotePort("MemPort")
			writeToBottom.Address = 0x10040
			writeToBottom.TrafficBytes = 12
			writeToBottom.TrafficClass = "memprotocol.WriteReq"

			t.State = State{
				InflightReqToBottom: []reqToBottomState{
					{
						ReqFromTopID:    readFromTop.ID,
						ReqFromTopSrc:   readFromTop.Src,
						ReqFromTopDst:   readFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", readFromTop),
						ReqToBottomID:   readToBottom.ID,
						ReqToBottomSrc:  readToBottom.Src,
						ReqToBottomDst:  readToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", readToBottom),
					},
					{
						ReqFromTopID:    writeFromTop.ID,
						ReqFromTopSrc:   writeFromTop.Src,
						ReqFromTopDst:   writeFromTop.Dst,
						ReqFromTopType:  fmt.Sprintf("%T", writeFromTop),
						ReqToBottomID:   writeToBottom.ID,
						ReqToBottomSrc:  writeToBottom.Src,
						ReqToBottomDst:  writeToBottom.Dst,
						ReqToBottomType: fmt.Sprintf("%T", writeToBottom),
					},
				},
			}
		})

		It("should do nothing if there is no response to process", func() {
			madeProgress := tRespondPipeMW.respond()
			Expect(madeProgress).To(BeFalse())
		})

		It("should respond data ready", func() {
			dataReady := memprotocol.DataReadyRsp{}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "memprotocol.DataReadyRsp"
			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())

			sent := topPort.RetrieveOutgoing()
			dr := sent.(memprotocol.DataReadyRsp)
			Expect(dr.RspTo).To(Equal(readFromTop.ID))
			Expect(dr.Data).To(Equal(dataReady.Data))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should respond write done", func() {
			done := memprotocol.WriteDoneRsp{}
			done.ID = timing.GetIDGenerator().Generate()
			done.RspTo = writeToBottom.ID
			done.TrafficBytes = 4
			done.TrafficClass = "memprotocol.WriteDoneRsp"
			bottomPort.Deliver(done)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeTrue())

			sent := topPort.RetrieveOutgoing()
			doneMsg := sent.(memprotocol.WriteDoneRsp)
			Expect(doneMsg.RspTo).To(Equal(writeFromTop.ID))

			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(1))
		})

		It("should stall if TopPort is busy", func() {
			dataReady := memprotocol.DataReadyRsp{}
			dataReady.ID = timing.GetIDGenerator().Generate()
			dataReady.RspTo = readToBottom.ID
			dataReady.TrafficBytes = 4
			dataReady.TrafficClass = "memprotocol.DataReadyRsp"

			fillOutgoing(topPort, topBufSize)
			bottomPort.Deliver(dataReady)

			madeProgress := tRespondPipeMW.respond()

			Expect(madeProgress).To(BeFalse())
			updatedState := &t.State
			Expect(updatedState.InflightReqToBottom).To(HaveLen(2))
		})
	})

	Context("state serialization", func() {
		It("should pass ValidateState", func() {
			err := modeling.ValidateState(State{})
			Expect(err).To(Succeed())
		})
	})

	Context("when handling control messages", func() {
		var (
			flushReq   memcontrolprotocol.Req
			restartReq memcontrolprotocol.Req
		)

		BeforeEach(func() {
			flushReq = memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdFlush,
			}
			flushReq.ID = timing.GetIDGenerator().Generate()
			flushReq.Src = messaging.RemotePort("Agent")
			flushReq.Dst = ctrlPort.AsRemote()
			flushReq.TrafficBytes = 4
			flushReq.TrafficClass = "memcontrolprotocol.Req"
			restartReq = memcontrolprotocol.Req{
				Command: memcontrolprotocol.CmdReset,
			}
			restartReq.ID = timing.GetIDGenerator().Generate()
			restartReq.Src = messaging.RemotePort("Agent")
			restartReq.Dst = ctrlPort.AsRemote()
			restartReq.TrafficBytes = 4
			restartReq.TrafficClass = "memcontrolprotocol.Req"

			readToBottom := memprotocol.ReadReq{}
			readToBottom.ID = timing.GetIDGenerator().Generate()
			readToBottom.Address = 0x20040
			readToBottom.AccessByteSize = 4
			readToBottom.TrafficBytes = 12
			readToBottom.TrafficClass = "memprotocol.ReadReq"

			nextState := &t.State
			nextState.InflightReqToBottom = []reqToBottomState{
				{
					ReqToBottomID:   readToBottom.ID,
					ReqToBottomType: fmt.Sprintf("%T", readToBottom),
				},
			}
		})

		It("rejects Flush as unsupported", func() {
			ctrlPort.Deliver(flushReq)

			madeProgress := t.Tick()

			Expect(madeProgress).To(BeTrue())
			rspMsg := ctrlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
			rsp := rspMsg.(memcontrolprotocol.Rsp)
			Expect(rsp.Success).To(BeFalse())
			Expect(rsp.Error).To(Equal(memcontrolprotocol.ErrUnsupported))
		})

		It("clears in-flight state on Reset", func() {
			ctrlPort.Deliver(restartReq)

			madeProgress := t.Tick()

			Expect(madeProgress).To(BeTrue())
			rspMsg := ctrlPort.RetrieveOutgoing()
			Expect(rspMsg).To(BeAssignableToTypeOf(memcontrolprotocol.Rsp{}))
			rsp := rspMsg.(memcontrolprotocol.Rsp)
			Expect(rsp.Success).To(BeTrue())
			updatedState := &t.State
			Expect(updatedState.ControlState).To(Equal(memcontrolprotocol.StateEnabled))
			Expect(updatedState.InflightReqToBottom).To(BeEmpty())
		})

		It("freezes incoming traffic while paused", func() {
			read := memprotocol.ReadReq{}
			read.ID = timing.GetIDGenerator().Generate()
			read.Src = messaging.RemotePort("Agent")
			read.Dst = topPort.AsRemote()
			read.Address = 0x100
			read.AccessByteSize = 4
			read.TrafficBytes = 12
			read.TrafficClass = "memprotocol.ReadReq"

			t.State.ControlState = memcontrolprotocol.StatePaused
			t.State.InflightReqToBottom = nil
			topPort.Deliver(read)

			for range 5 {
				t.Tick()
			}

			Expect(topPort.PeekIncoming()).ToNot(BeNil())
			Expect(t.State.Transactions).To(BeEmpty())
			Expect(translationPort.RetrieveOutgoing()).To(BeNil())
		})
	})
})

// fillOutgoing fills a port's outgoing buffer with dummy messages so the next
// CanSend returns false.
func fillOutgoing(p messaging.Port, n int) {
	for i := 0; i < n; i++ {
		dummy := memprotocol.WriteDoneRsp{}
		dummy.ID = timing.GetIDGenerator().Generate()
		dummy.Src = p.AsRemote()
		dummy.Dst = messaging.RemotePort("Dummy")
		dummy.TrafficClass = "memprotocol.WriteDoneRsp"
		Expect(p.CanSend()).To(BeTrue())
		p.Send(dummy)
	}
}
