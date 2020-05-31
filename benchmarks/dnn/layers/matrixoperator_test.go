package layers_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/benchmarks/dnn/layers"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Matrix Operator", func() {
	var (
		gpuDriver *driver.Driver
		context   *driver.Context
		mo        *layers.MatrixOperator
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeR9NanoBuilder().Build()
		gpuDriver.Run()
		context = gpuDriver.Init()
		mo = layers.NewMatrixOperator(gpuDriver, context)
	})

	It("should do gemm", func() {
		a := mo.CreateMatrix(3, 2)
		mo.ToGPU(a, []float32{
			1, 2, 3,
			4, 5, 6,
		})

		b := mo.CreateMatrix(1, 3)
		mo.ToGPU(b, []float32{
			7,
			8,
			9,
		})

		c := mo.CreateMatrix(1, 2)
		mo.ToGPU(c, []float32{
			-1, -2,
		})

		d := mo.CreateMatrix(1, 2)

		mo.Gemm(false, false,
			2, 1, 3,
			0.1, 2.2,
			a, b, c, d)

		outData := make([]float32, 2)
		mo.FromGPU(d, outData)

		expectedOutData := []float32{2.8, 7.8}

		for i := range expectedOutData {
			Expect(outData[i]).To(
				BeNumerically("~", expectedOutData[i], 0.001))
		}
	})

	It("should transpose", func() {
		in := mo.CreateMatrix(2, 3)
		out := mo.CreateMatrix(3, 2)

		inData := []float32{
			1, 2, 3,
			4, 5, 6,
		}
		mo.ToGPU(in, inData)

		mo.Transpose(in, out)

		outData := make([]float32, 3*2)
		mo.FromGPU(out, outData)

		Expect(outData).To(Equal([]float32{
			1, 4,
			2, 5,
			3, 6,
		}))
	})
})
