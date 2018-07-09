package gcn3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

var _ = Describe("CommandProcessor", func() {

	var (
		engine           *core.MockEngine
		driver           *core.MockComponent
		dispatcher       *core.MockComponent
		commandProcessor *CommandProcessor
		connection       *core.MockConnection
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		connection = core.NewMockConnection()

		driver = core.NewMockComponent("dispatcher")
		dispatcher = core.NewMockComponent("dispatcher")
		commandProcessor = NewCommandProcessor("commandProcessor", engine)

		commandProcessor.Dispatcher = dispatcher.ToOutside
		commandProcessor.Driver = driver.ToOutside

		connection.PlugIn(commandProcessor.ToDispatcher)
		connection.PlugIn(commandProcessor.ToDriver)
	})

	It("should forward kernel launching request to Dispatcher", func() {
		req := kernels.NewLaunchKernelReq()
		req.SetSrc(driver.ToOutside)
		req.SetDst(commandProcessor.ToDriver)

		reqExpect := kernels.NewLaunchKernelReq()
		reqExpect.SetSrc(commandProcessor.ToDispatcher)
		reqExpect.SetDst(dispatcher.ToOutside)

		connection.ExpectSend(reqExpect, nil)

		commandProcessor.Handle(req)

		Expect(connection.AllExpectedSent()).To(BeTrue())
	})

	It("should delay forward kernel launching request to the Driver", func() {
		req := kernels.NewLaunchKernelReq()
		req.SetSrc(dispatcher.ToOutside)
		req.SetDst(commandProcessor.ToDispatcher)

		commandProcessor.Handle(req)

		Expect(engine.ScheduledEvent).To(HaveLen(1))
	})
})
