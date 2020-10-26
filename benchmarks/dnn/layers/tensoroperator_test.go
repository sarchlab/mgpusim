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

	It("should transpose", func() {
		in := to.CreateTensor([]int{2, 3})
		out := to.CreateTensor([]int{3, 2})

		inData := []float32{
			1, 2, 3,
			4, 5, 6,
		}
		to.ToGPU(in, inData)

		to.Transpose(in, out)

		outData := make([]float32, 3*2)
		to.FromGPU(out, outData)

		Expect(outData).To(Equal([]float32{
			1, 4,
			2, 5,
			3, 6,
		}))
	})
})
