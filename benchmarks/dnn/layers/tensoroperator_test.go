package layers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Tensor Operator", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		to        *TensorOperator
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeEmuBuilder().
			WithoutProgressBar().
			Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		to = NewTensorOperator(gpuDriver, context)
	})

	AfterEach(func() {
		gpuDriver.Terminate()
	})

	It("should do gemm", func() {
		a := to.CreateTensor([]int{3, 2})
		to.ToGPU(a, []float32{
			1, 2, 3,
			4, 5, 6,
		})

		b := to.CreateTensor([]int{1, 3})
		to.ToGPU(b, []float32{
			7,
			8,
			9,
		})

		c := to.CreateTensor([]int{1, 2})
		to.ToGPU(c, []float32{
			-1, -2,
		})

		d := to.CreateTensor([]int{1, 2})

		to.Gemm(false, false,
			2, 1, 3,
			0.1, 2.2,
			a, b, c, d)

		outData := make([]float32, 2)
		to.FromGPU(d, outData)

		expectedOutData := []float32{2.8, 7.8}

		for i := range expectedOutData {
			Expect(outData[i]).To(
				BeNumerically("~", expectedOutData[i], 0.001))
		}
	})

	It("should transpose matrix", func() {
		in := to.CreateTensor([]int{2, 3})
		out := to.CreateTensor([]int{3, 2})

		inData := []float32{
			1, 2, 3,
			4, 5, 6,
		}
		to.ToGPU(in, inData)

		to.TransposeMatrix(in, out)

		outData := make([]float32, 3*2)
		to.FromGPU(out, outData)

		Expect(outData).To(Equal([]float32{
			1, 4,
			2, 5,
			3, 6,
		}))
	})

	FIt("should do general transpose", func() {
		in := to.CreateTensor([]int{2, 4, 3, 3})
		inData := []float32{
			1.111, 1.112, 1.113,
			1.121, 1.122, 1.123,
			1.131, 1.132, 1.133,

			1.211, 1.212, 1.213,
			1.221, 1.222, 1.223,
			1.231, 1.232, 1.233,

			1.311, 1.312, 1.313,
			1.321, 1.322, 1.323,
			1.331, 1.332, 1.333,

			1.411, 1.412, 1.413,
			1.421, 1.422, 1.423,
			1.431, 1.432, 1.433,

			2.111, 2.112, 2.113,
			2.121, 2.122, 2.123,
			2.131, 2.132, 2.133,

			2.211, 2.212, 2.213,
			2.221, 2.222, 2.223,
			2.231, 2.232, 2.233,

			2.311, 2.312, 2.313,
			2.321, 2.322, 2.323,
			2.331, 2.332, 2.333,

			2.411, 2.412, 2.413,
			2.421, 2.422, 2.423,
			2.431, 2.432, 2.433,
		}
		to.ToGPU(in, inData)

		outData := []float32{
			1.111, 1.112, 1.113,
			1.121, 1.122, 1.123,
			1.131, 1.132, 1.133,

			2.111, 2.112, 2.113,
			2.121, 2.122, 2.123,
			2.131, 2.132, 2.133,

			1.211, 1.212, 1.213,
			1.221, 1.222, 1.223,
			1.231, 1.232, 1.233,

			2.211, 2.212, 2.213,
			2.221, 2.222, 2.223,
			2.231, 2.232, 2.233,

			1.311, 1.312, 1.313,
			1.321, 1.322, 1.323,
			1.331, 1.332, 1.333,

			2.311, 2.312, 2.313,
			2.321, 2.322, 2.323,
			2.331, 2.332, 2.333,

			1.411, 1.412, 1.413,
			1.421, 1.422, 1.423,
			1.431, 1.432, 1.433,

			2.411, 2.412, 2.413,
			2.421, 2.422, 2.423,
			2.431, 2.432, 2.433,
		}

		out := to.CreateTensor([]int{4, 2, 3, 3})
		to.TransposeTensor(in, out, []int{1, 0, 2, 3})
		outV := out.Vector()

		for i := range outData {
			Expect(outV[i]).To(BeNumerically("~", outData[i], 1e-3))
		}
	})
})
