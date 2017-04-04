package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

var _ = Describe("ComputeUnit", func() {

	var (
		mockDispatcher *core.MockComponent
		connection     *core.DirectConnection
		cu             *emu.ComputeUnit
	)

	BeforeEach(func() {
		mockDispatcher = core.NewMockComponent("MockDispatcher")
		mockDispatcher.AddPort("ToCU")
		connection = core.NewDirectConnection()

		cu = emu.NewComputeUnit("cu", nil, nil, nil, nil, nil)

		core.PlugIn(mockDispatcher, "ToCU", connection)
		core.PlugIn(cu, "ToDispatcher", connection)
	})

	Context("on MapWorkGroupReq", func() {
		It("should panic if there is workgroup executing", func() {
			cu.WG = kernels.NewWorkGroup()
			req := emu.NewMapWGReq()
			Expect(func() { cu.Receive(req) }).To(Panic())
		})
	})

	Context("on Read and write registers", func() {
		It("should read and write vgrs", func() {
			cu.WriteReg(insts.VReg(0), 0, []byte{0, 1, 2, 3})
			res := cu.ReadReg(insts.VReg(0), 0, 4)
			Expect(res).To(ConsistOf([]byte{0, 1, 2, 3}))
		})

		It("should read and write sgrs", func() {
			cu.WriteReg(insts.SReg(0), 0, []byte{0, 1, 2, 3})
			res := cu.ReadReg(insts.SReg(0), 0, 4)
			Expect(res).To(ConsistOf([]byte{0, 1, 2, 3}))
		})
	})
})
