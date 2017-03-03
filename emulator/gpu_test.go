package emulator_test

import (
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/gcn3/emulator"
)

var _ = ginkgo.Describe("Gpu (unit test)", func() {

	var (
		commandProcessor *conn.MockComponent
		gpu              *emulator.Gpu
		driver           *conn.MockComponent
		connection       *conn.DirectConnection
	)

	ginkgo.BeforeEach(func() {
		commandProcessor = conn.NewMockComponent("CommandProcessor")
		gpu = emulator.NewGpu("gpu")
		driver = conn.NewMockComponent("Driver")

		gpu.Driver = driver
		gpu.CommandProcessor = commandProcessor

		driver.AddPort("ToGPU")
		commandProcessor.AddPort("ToDriver")

		connection = conn.NewDirectConnection()
		conn.PlugIn(commandProcessor, "ToDriver", connection)
		conn.PlugIn(gpu, "ToDriver", connection)
		conn.PlugIn(gpu, "ToCommandProcessor", connection)
		conn.PlugIn(driver, "ToGPU", connection)
	})

	ginkgo.It("Should forward all request to CommandProcessor", func() {
		req := conn.NewBasicRequest()
		req.SetSource(driver)
		req.SetDestination(gpu)

		commandProcessor.ToReceiveReq(req, nil)

		err := connection.Send(req)
		Expect(err).To(BeNil())

		Expect(req.Source()).To(BeIdenticalTo(gpu))
		Expect(req.Destination()).To(BeIdenticalTo(commandProcessor))
	})
})
