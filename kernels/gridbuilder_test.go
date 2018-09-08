package kernels_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
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
		Expect(grid.WorkGroups[0].CurrSizeX).To(Equal(256))
		Expect(grid.WorkGroups[1].CurrSizeX).To(Equal(256))
		Expect(grid.WorkGroups[2].CurrSizeX).To(Equal(256))
		Expect(grid.WorkGroups[3].CurrSizeX).To(Equal(256))
		Expect(grid.WorkGroups[4].CurrSizeX).To(Equal(1))
		for i := 0; i < 5; i++ {
			Expect(grid.WorkGroups[i].SizeX).To(Equal(256))
		}

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
		for i := 0; i < 16; i++ {
			Expect(grid.WorkGroups[i].CurrSizeX).To(Equal(16))
			Expect(grid.WorkGroups[i].CurrSizeY).To(Equal(16))
			Expect(grid.WorkGroups[i].CurrSizeZ).To(Equal(4))
		}
		for i := 16; i < 20; i++ {
			Expect(grid.WorkGroups[i].CurrSizeX).To(Equal(16))
			Expect(grid.WorkGroups[i].CurrSizeY).To(Equal(16))
			Expect(grid.WorkGroups[i].CurrSizeZ).To(Equal(1))
		}
	})

	It("should build 3D grid when x is partial", func() {
		codeObject := new(insts.HsaCo)

		packet := new(kernels.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 16
		packet.WorkgroupSizeY = 16
		packet.WorkgroupSizeZ = 4
		packet.GridSizeX = 33
		packet.GridSizeY = 31
		packet.GridSizeZ = 17

		req := kernels.NewLaunchKernelReq()
		req.HsaCo = codeObject
		req.Packet = packet

		grid := builder.Build(req)

		expectedXSize := []int{16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1, 16, 16, 1}
		expectedYSize := []int{16, 16, 16, 15, 15, 15, 16, 16, 16, 15, 15, 15, 16, 16, 16, 15, 15, 15, 16, 16, 16, 15, 15, 15, 16, 16, 16, 15, 15, 15}
		expectedZSize := []int{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 1, 1, 1, 1, 1, 1}

		Expect(len(grid.WorkGroups)).To(Equal(30))

		for i := 0; i < 30; i++ {
			Expect(grid.WorkGroups[i].CurrSizeX).To(Equal(expectedXSize[i]))
			Expect(grid.WorkGroups[i].CurrSizeY).To(Equal(expectedYSize[i]))
			Expect(grid.WorkGroups[i].CurrSizeZ).To(Equal(expectedZSize[i]))
		}

	})

})
