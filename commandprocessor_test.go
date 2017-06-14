package gcn3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
)

var _ = Describe("CommandProcessor", func() {

	var (
		driver           *core.MockComponent
		dispatcher       *core.MockComponent
		commandProcessor *CommandProcessor
		connection       *core.MockConnection
	)

	BeforeEach(func() {
		connection = core.NewMockConnection()

		driver = core.NewMockComponent("dispatcher")
		dispatcher = core.NewMockComponent("dispatcher")
		commandProcessor = NewCommandProcessor("commandProcessor")

		commandProcessor.Dispatcher = dispatcher
		commandProcessor.Driver = driver

		core.PlugIn(commandProcessor, "ToDispatcher", connection)
	})

	It("should forward kernel launching request to Dispatcher", func() {
		req := kernels.NewLaunchKernelReq()
		req.SetSrc(driver)
		req.SetDst(commandProcessor)

		reqExpect := kernels.NewLaunchKernelReq()
		reqExpect.SetSrc(commandProcessor)
		reqExpect.SetDst(dispatcher)

		connection.ExpectSend(reqExpect, nil)

		commandProcessor.Recv(req)

		Expect(connection.AllExpectedSent()).To(BeTrue())
	})

	It("should forward kernel launching request to the Driver", func() {
		req := kernels.NewLaunchKernelReq()
		req.SetSrc(dispatcher)
		req.SetDst(commandProcessor)

		reqExpect := kernels.NewLaunchKernelReq()
		reqExpect.SetSrc(commandProcessor)
		reqExpect.SetDst(driver)

		connection.ExpectSend(reqExpect, nil)

		commandProcessor.Recv(req)

		Expect(connection.AllExpectedSent()).To(BeTrue())

	})
})
