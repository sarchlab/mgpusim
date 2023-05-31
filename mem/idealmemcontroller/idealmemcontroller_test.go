package idealmemcontroller

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"

	. "github.com/onsi/gomega"
)

var _ = Describe("Ideal Memory Controller", func() {

	var (
		mockCtrl      *gomock.Controller
		engine        *MockEngine
		memController *Comp
		port          *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		port = NewMockPort(mockCtrl)

		memController = New("MemCtrl", engine, 1*mem.MB)
		memController.Freq = 1000 * sim.MHz
		memController.Latency = 10
		memController.topPort = port
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should stall if too many transactions are running", func() {
		memController.currNumTransaction = 8

		madeProgress := memController.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should process read request", func() {
		readReq := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithByteSize(4).
			Build()
		port.EXPECT().Retrieve(gomock.Any()).Return(readReq)

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		madeProgress := memController.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(memController.currNumTransaction).To(Equal(1))
	})

	It("should process write request", func() {
		writeReq := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithData([]byte{0, 1, 2, 3}).
			WithDirtyMask([]bool{false, false, true, false}).
			Build()
		port.EXPECT().Retrieve(gomock.Any()).Return(writeReq)

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		madeProgress := memController.Tick(10)
		Expect(madeProgress).To(BeTrue())
		Expect(memController.currNumTransaction).To(Equal(1))
	})

	It("should handle read respond event", func() {
		data := []byte{1, 2, 3, 4}
		memController.Storage.Write(0, data)
		memController.currNumTransaction = 1

		readReq := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithByteSize(4).
			Build()

		event := newReadRespondEvent(11, memController, readReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{}))

		memController.Handle(event)

		Expect(memController.currNumTransaction).To(Equal(0))
	})

	It("should retry read if send DataReady failed", func() {
		data := []byte{1, 2, 3, 4}
		memController.Storage.Write(0, data)

		readReq := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithByteSize(4).
			Build()
		event := newReadRespondEvent(11, memController, readReq)

		port.EXPECT().
			Send(gomock.AssignableToTypeOf(&mem.DataReadyRsp{})).
			Return(&sim.SendError{})

		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&readRespondEvent{}))

		memController.Handle(event)
	})

	It("should handle write respond event without write mask", func() {
		data := []byte{1, 2, 3, 4}
		writeReq := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithData(data).
			Build()
		event := newWriteRespondEvent(11, memController, writeReq)
		memController.currNumTransaction = 1

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{}))

		memController.Handle(event)

		retData, _ := memController.Storage.Read(0, 4)
		Expect(retData).To(Equal([]byte{1, 2, 3, 4}))
		Expect(memController.currNumTransaction).To(Equal(0))
	})

	It("should handle write respond event", func() {
		memController.Storage.Write(0, []byte{9, 9, 9, 9})
		data := []byte{1, 2, 3, 4}
		dirtyMask := []bool{false, true, false, false}

		writeReq := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithData(data).
			WithDirtyMask(dirtyMask).
			Build()
		event := newWriteRespondEvent(11, memController, writeReq)

		engine.EXPECT().Schedule(gomock.Any())
		port.EXPECT().Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{}))

		memController.Handle(event)
		retData, _ := memController.Storage.Read(0, 4)
		Expect(retData).To(Equal([]byte{9, 2, 9, 9}))
	})

	Measure("write with write mask", func(b Benchmarker) {
		data := make([]byte, 64)
		dirtyMask := []bool{
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
			true, true, true, true, false, false, false, false,
		}
		writeReq := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithData(data).
			WithDirtyMask(dirtyMask).
			Build()

		event := newWriteRespondEvent(11, memController, writeReq)
		engine.EXPECT().Schedule(gomock.Any()).AnyTimes()
		port.EXPECT().
			Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{})).
			AnyTimes()

		b.Time("write time", func() {
			for i := 0; i < 100000; i++ {
				memController.Handle(event)
			}
		})
	}, 100)

	It("should retry write respond event, if network busy", func() {
		data := []byte{1, 2, 3, 4}

		writeReq := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithDst(memController.topPort).
			WithAddress(0).
			WithData(data).
			Build()
		event := newWriteRespondEvent(11, memController, writeReq)

		port.EXPECT().
			Send(gomock.AssignableToTypeOf(&mem.WriteDoneRsp{})).
			Return(&sim.SendError{})
		engine.EXPECT().
			Schedule(gomock.AssignableToTypeOf(&writeRespondEvent{}))

		memController.Handle(event)
	})
})
