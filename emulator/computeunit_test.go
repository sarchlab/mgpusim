package emulator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/gcn3/emulator"
)

var _ = Describe("ComputeUnit", func() {

	var (
		mockDispatcher *conn.MockComponent
		connection     *conn.DirectConnection
		cu             *emulator.ComputeUnit
	)

	BeforeEach(func() {
		mockDispatcher = conn.NewMockComponent("MockDispatcher")
		mockDispatcher.AddPort("ToCU")
		connection = conn.NewDirectConnection()
		cu = emulator.NewComputeUnit("cu")

		conn.PlugIn(mockDispatcher, "ToCU", connection)
		conn.PlugIn(cu, "ToDispatcher", connection)
	})

	Context("on MapWorkGroupReq", func() {
		It("should reject if there is workgroup executing", func() {
			cu.WorkGroup = emulator.NewWorkGroup()

			req := emulator.NewMapWorkGroupReq()
			req.SetSource(mockDispatcher)
			req.SetDestination(cu)

			mockDispatcher.ToReceiveReq(req, nil)

			cu.Receive(req)

			Expect(mockDispatcher.AllReqReceived()).To(BeTrue())
			Expect(req.IsReply).To(BeTrue())
			Expect(req.Succeed).NotTo(BeTrue())
		})
	})
})
