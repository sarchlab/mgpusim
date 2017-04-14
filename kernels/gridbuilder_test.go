package kernels_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

var _ = Describe("GridBuilder", func() {

	var (
		builder *kernels.GridBuilderImpl
	)

	BeforeEach(func() {
		builder = new(kernels.GridBuilderImpl)
	})

	It("should build 1D grid", func() {

		codeObject := new(insts.HsaCo)

		packet := new(kernels.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 256
		packet.WorkgroupSizeY = 1
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 1025
		packet.GridSizeY = 1
		packet.GridSizeZ = 1

		req := kernels.NewLaunchKernelReq()
		req.HsaCo = codeObject
		req.Packet = packet

		grid := builder.Build(req)

		Expect(len(grid.WorkGroups)).To(Equal(5))
		Expect(grid.WorkGroups[0].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[1].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[2].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[3].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[4].SizeX).To(Equal(1))

	})

	It("should build 2D grid", func() {

		codeObject := new(insts.HsaCo)

		packet := new(kernels.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 16
		packet.WorkgroupSizeY = 16
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 1024
		packet.GridSizeY = 1025
		packet.GridSizeZ = 1

		req := kernels.NewLaunchKernelReq()
		req.HsaCo = codeObject
		req.Packet = packet

		grid := builder.Build(req)

		Expect(len(grid.WorkGroups)).To(Equal(4096 + 64))
	})

	It("should build 3D grid", func() {
		codeObject := new(insts.HsaCo)

		packet := new(kernels.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 16
		packet.WorkgroupSizeY = 16
		packet.WorkgroupSizeZ = 4
		packet.GridSizeX = 32
		packet.GridSizeY = 32
		packet.GridSizeZ = 17

		req := kernels.NewLaunchKernelReq()
		req.HsaCo = codeObject
		req.Packet = packet

		grid := builder.Build(req)

		Expect(len(grid.WorkGroups)).To(Equal(20))
	})

})
