package kernels

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"gitlab.com/akita/gcn3/insts"
)

var _ = Describe("GridBuilder", func() {

	var (
		builder *gridBuilderImpl
	)

	BeforeEach(func() {
		builder = &gridBuilderImpl{}
	})

	It("should build 1D grid workgroup", func() {
		codeObject := new(insts.HsaCo)
		packet := new(HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 256
		packet.WorkgroupSizeY = 1
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 1025
		packet.GridSizeY = 1
		packet.GridSizeZ = 1
		builder.SetKernel(codeObject, packet)

		wg1 := builder.NextWG()
		wg2 := builder.NextWG()
		wg3 := builder.NextWG()
		wg4 := builder.NextWG()
		wg5 := builder.NextWG()
		wg6 := builder.NextWG()

		Expect(builder.NumWG()).To(Equal(5))
		Expect(wg1.SizeX).To(Equal(256))
		Expect(wg1.SizeY).To(Equal(1))
		Expect(wg1.SizeZ).To(Equal(1))
		Expect(wg1.IDX).To(Equal(0))
		Expect(wg1.IDY).To(Equal(0))
		Expect(wg1.IDZ).To(Equal(0))
		Expect(wg1.Wavefronts).To(HaveLen(4))
		Expect(wg1.WorkItems).To(HaveLen(256))
		Expect(wg2.SizeX).To(Equal(256))
		Expect(wg2.SizeY).To(Equal(1))
		Expect(wg2.SizeZ).To(Equal(1))
		Expect(wg2.IDX).To(Equal(1))
		Expect(wg2.IDY).To(Equal(0))
		Expect(wg2.IDZ).To(Equal(0))
		Expect(wg3.IDX).To(Equal(2))
		Expect(wg4.IDX).To(Equal(3))
		Expect(wg5.IDX).To(Equal(4))
		Expect(wg5.CurrSizeX).To(Equal(1))
		Expect(wg6).To(BeNil())
	})

	It("should build 2D grid", func() {
		codeObject := new(insts.HsaCo)
		packet := new(HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 16
		packet.WorkgroupSizeY = 16
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 33
		packet.GridSizeY = 17
		packet.GridSizeZ = 1
		builder.SetKernel(codeObject, packet)

		wg1 := builder.NextWG()
		wg2 := builder.NextWG()
		wg3 := builder.NextWG()
		wg4 := builder.NextWG()
		wg5 := builder.NextWG()
		wg6 := builder.NextWG()
		wg7 := builder.NextWG()

		Expect(builder.NumWG()).To(Equal(6))

		Expect(wg1.SizeX).To(Equal(16))
		Expect(wg1.SizeY).To(Equal(16))
		Expect(wg1.SizeZ).To(Equal(1))
		Expect(wg1.IDX).To(Equal(0))
		Expect(wg1.IDY).To(Equal(0))
		Expect(wg1.IDZ).To(Equal(0))
		Expect(wg1.Wavefronts).To(HaveLen(4))
		Expect(wg1.WorkItems).To(HaveLen(256))

		Expect(wg2.SizeX).To(Equal(16))
		Expect(wg2.SizeY).To(Equal(16))
		Expect(wg2.SizeZ).To(Equal(1))
		Expect(wg2.IDX).To(Equal(1))
		Expect(wg2.IDY).To(Equal(0))
		Expect(wg2.IDZ).To(Equal(0))

		Expect(wg3.IDX).To(Equal(2))
		Expect(wg3.IDY).To(Equal(0))
		Expect(wg3.CurrSizeX).To(Equal(1))
		Expect(wg3.CurrSizeY).To(Equal(16))

		Expect(wg4.IDX).To(Equal(0))
		Expect(wg4.IDY).To(Equal(1))
		Expect(wg4.CurrSizeX).To(Equal(16))
		Expect(wg4.CurrSizeY).To(Equal(1))

		Expect(wg5.IDX).To(Equal(1))
		Expect(wg5.IDY).To(Equal(1))
		Expect(wg5.CurrSizeX).To(Equal(16))
		Expect(wg5.CurrSizeY).To(Equal(1))

		Expect(wg6.IDX).To(Equal(2))
		Expect(wg6.IDY).To(Equal(1))
		Expect(wg6.CurrSizeX).To(Equal(1))
		Expect(wg6.CurrSizeY).To(Equal(1))

		Expect(wg7).To(BeNil())
	})
})
