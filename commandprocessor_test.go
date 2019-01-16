package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"

	// . "github.com/onsi/gomega"
	"gitlab.com/akita/akita/mock_akita"
)

var _ = Describe("CommandProcessor", func() {

	var (
		mockCtrl         *gomock.Controller
		engine           *mock_akita.MockEngine
		driver           *mock_akita.MockPort
		dispatcher       *mock_akita.MockPort
		commandProcessor *CommandProcessor
		toDriver         *mock_akita.MockPort
		toDispatcher     *mock_akita.MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = mock_akita.NewMockEngine(mockCtrl)

		driver = mock_akita.NewMockPort(mockCtrl)
		dispatcher = mock_akita.NewMockPort(mockCtrl)
		toDriver = mock_akita.NewMockPort(mockCtrl)
		toDispatcher = mock_akita.NewMockPort(mockCtrl)
		commandProcessor = NewCommandProcessor("commandProcessor", engine)
		commandProcessor.ToDispatcher = toDispatcher
		commandProcessor.ToDriver = toDriver

		commandProcessor.Dispatcher = dispatcher
		commandProcessor.Driver = driver
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should forward kernel launching request to Dispatcher", func() {
		req := NewLaunchKernelReq(10,
			driver, commandProcessor.ToDriver)
		req.SetEventTime(10)

		toDispatcher.EXPECT().Send(gomock.AssignableToTypeOf(req))

		commandProcessor.Handle(req)
	})

	It("should delay forward kernel launching request to the Driver", func() {
		req := NewLaunchKernelReq(10,
			dispatcher, commandProcessor.ToDispatcher)

		engine.EXPECT().Schedule(
			gomock.AssignableToTypeOf(&ReplyKernelCompletionEvent{}))

		commandProcessor.Handle(req)
	})
})
