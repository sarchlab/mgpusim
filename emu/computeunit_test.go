package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = Describe("ComputeUnit", func() {

	var (
		mockDispatcher *conn.MockComponent
		connection     *conn.DirectConnection
		cu             *emu.ComputeUnit
	)

	BeforeEach(func() {
		mockDispatcher = conn.NewMockComponent("MockDispatcher")
		mockDispatcher.AddPort("ToCU")
		connection = conn.NewDirectConnection()
		cu = emu.NewComputeUnit("cu", nil, nil, nil)

		conn.PlugIn(mockDispatcher, "ToCU", connection)
		conn.PlugIn(cu, "ToDispatcher", connection)
	})

	Context("on MapWorkGroupReq", func() {
		It("should reject if there is workgroup executing", func() {
			cu.WG = emu.NewWorkGroup()

			req := emu.NewMapWGReq()
			req.SetSource(mockDispatcher)
			req.SetDestination(cu)

			mockDispatcher.ToReceiveReq(req, nil)

			cu.Receive(req)

			Expect(mockDispatcher.AllReqReceived()).To(BeTrue())
			Expect(req.IsReply).To(BeTrue())
			Expect(req.Succeed).NotTo(BeTrue())
		})
	})

	Context("on Read and write registers", func() {
		It("should read and write vgrs", func() {
			cu.WriteReg(disasm.VReg(0), 0, []byte{0, 1, 2, 3})
			res := cu.ReadReg(disasm.VReg(0), 0, 4)
			Expect(res).To(ConsistOf([]byte{0, 1, 2, 3}))
		})

		It("should read and write sgrs", func() {
			cu.WriteReg(disasm.SReg(0), 0, []byte{0, 1, 2, 3})
			res := cu.ReadReg(disasm.SReg(0), 0, 4)
			Expect(res).To(ConsistOf([]byte{0, 1, 2, 3}))
		})

	})
})
