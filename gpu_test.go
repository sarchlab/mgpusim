package gcn3_test

//import (
//	"github.com/onsi/ginkgo"
//	. "github.com/onsi/gomega"
//	"gitlab.com/yaotsu/core"
//	"gitlab.com/yaotsu/gcn3"
//)

//var _ = ginkgo.Describe("Gpu (unit test)", func() {
//
//	var (
//		commandProcessor *core.MockComponent
//		gpu              *gcn3.Gpu
//		driver           *core.MockComponent
//		connection       *core.DirectConnection
//	)
//
//	ginkgo.BeforeEach(func() {
//		commandProcessor = core.NewMockComponent("CommandProcessor")
//		gpu = gcn3.NewGpu("gpu")
//		driver = core.NewMockComponent("Driver")
//
//		gpu.Driver = driver
//		gpu.CommandProcessor = commandProcessor
//
//		driver.AddPort("ToGPU")
//		commandProcessor.AddPort("ToDriver")
//
//		connection = core.NewDirectConnection()
//		core.PlugIn(commandProcessor, "ToDriver", connection)
//		core.PlugIn(gpu, "ToDriver", connection)
//		core.PlugIn(gpu, "ToCommandProcessor", connection)
//		core.PlugIn(driver, "ToGPU", connection)
//	})
//
//	ginkgo.It("Should forward all request to CommandProcessor", func() {
//		req := core.NewReqBase()
//		req.SetSrc(driver)
//		req.SetDst(gpu)
//
//		commandProcessor.ToReceiveReq(req, nil)
//
//		err := connection.Send(req)
//		Expect(err).To(BeNil())
//
//		Expect(req.Src()).To(BeIdenticalTo(gpu))
//		Expect(req.Dst()).To(BeIdenticalTo(commandProcessor))
//	})
//})
