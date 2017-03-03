package emulator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/gcn3/emulator"
)

var _ = Describe("CommandProcessor", func() {

	var (
		driver           *conn.MockComponent
		dispatcher       *conn.MockComponent
		commandProcessor *emulator.CommandProcessor
		connection       *conn.DirectConnection
	)

	BeforeEach(func() {
		connection = conn.NewDirectConnection()

		driver = conn.NewMockComponent("dispatcher")
		driver.AddPort("ToGPU")
		dispatcher = conn.NewMockComponent("dispatcher")
		dispatcher.AddPort("ToCommandProcessor")
		commandProcessor = emulator.NewCommandProcessor("commandProcessor")

		commandProcessor.Dispatcher = dispatcher

		conn.PlugIn(dispatcher, "ToCommandProcessor", connection)
		conn.PlugIn(commandProcessor, "ToDispatcher", connection)
		conn.PlugIn(driver, "ToGPU", connection)
	})

	It("should forward kernel launching request to Dispatcher", func() {
		req := emulator.NewLaunchKernelReq()
		req.SetSource(driver)
		req.SetDestination(commandProcessor)

		dispatcher.ToReceiveReq(req, nil)

		commandProcessor.Receive(req)

		Expect(dispatcher.AllReqReceived()).To(BeTrue())
	})
})
