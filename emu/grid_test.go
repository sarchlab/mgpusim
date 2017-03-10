package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
)

var _ = Describe("Grid", func() {

	It("should spawn workgroups, 1D", func() {
		grid := emu.NewGrid()

		codeObject := new(disasm.HsaCo)
		grid.CodeObject = codeObject

		packet := new(emu.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 256
		packet.WorkgroupSizeY = 1
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 1025
		packet.GridSizeY = 1
		packet.GridSizeZ = 1
		grid.Packet = packet

		grid.SpawnWorkGroups()

		Expect(len(grid.WorkGroups)).To(Equal(5))
		Expect(grid.WorkGroups[0].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[1].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[2].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[3].SizeX).To(Equal(256))
		Expect(grid.WorkGroups[4].SizeX).To(Equal(1))

	})

	It("should spawn workgroups, 2D", func() {
		grid := emu.NewGrid()

		codeObject := new(disasm.HsaCo)
		grid.CodeObject = codeObject

		packet := new(emu.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 16
		packet.WorkgroupSizeY = 16
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 1024
		packet.GridSizeY = 1025
		packet.GridSizeZ = 1
		grid.Packet = packet

		grid.SpawnWorkGroups()

		Expect(len(grid.WorkGroups)).To(Equal(4096 + 64))
	})

	It("should spawn workgroups, 3D", func() {
		grid := emu.NewGrid()

		codeObject := new(disasm.HsaCo)
		grid.CodeObject = codeObject

		packet := new(emu.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 16
		packet.WorkgroupSizeY = 16
		packet.WorkgroupSizeZ = 4
		packet.GridSizeX = 32
		packet.GridSizeY = 32
		packet.GridSizeZ = 17
		grid.Packet = packet

		grid.SpawnWorkGroups()

		Expect(len(grid.WorkGroups)).To(Equal(20))
	})

})
