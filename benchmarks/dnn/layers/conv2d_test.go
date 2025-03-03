package layers

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/benchmarks/dnn/tensor"
)

func tensorMatch(actual, expected tensor.Tensor) {
	Expect(actual.Size()).To(Equal(expected.Size()))
	Expect(actual.Descriptor()).To(Equal(expected.Descriptor()))

	actualData := actual.Vector()
	expectedData := expected.Vector()

	for i := range expectedData {
		Expect(actualData[i]).To(BeNumerically("~", expectedData[i], 1e-3))
	}
}

var _ = Describe("Conv2d", func() {
	var (
		layer *Conv2D
		to    tensor.CPUOperator
	)

	BeforeEach(func() {
		to = tensor.CPUOperator{}
	})

	It("should forward", func() {
		layer = NewConv2D(
			1,
			to,
			[]int{1, 3, 3}, []int{1, 1, 3, 3}, []int{1, 1}, []int{1, 1})

		input := to.CreateWithData(
			[]float64{
				1.1, 1.2, 1.3,
				2.1, 2.2, 2.3,
				3.1, 3.2, 3.3,
			},
			[]int{1, 1, 3, 3},
			"NCHW")

		to.Init(layer.weights,
			[]float64{
				1.1, 1.2, 1.3,
				2.1, 2.2, 2.3,
				3.1, 3.2, 3.3,
			})
		to.Init(layer.bias, []float64{1.0})

		output := layer.Forward(input)

		Expect(output.Descriptor()).To(Equal("NCHW"))
		Expect(output.Size()).To(Equal([]int{1, 1, 3, 3}))

		outputVector := output.Vector()
		expectedOutputVector := []float64{
			20.16, 30.08, 20.56,
			34.04, 50.62, 34.04,
			20.56, 30.08, 20.16,
		}
		for i := range expectedOutputVector {
			Expect(outputVector[i]).
				To(BeNumerically("~", expectedOutputVector[i], 1e-3))
		}
	})

	It("should forward with stride", func() {
		layer = NewConv2D(
			1,
			to,
			[]int{1, 5, 5}, []int{1, 1, 3, 3}, []int{2, 2}, []int{1, 1})

		input := to.CreateWithData(
			[]float64{
				1.1, 1.2, 1.3, 1.4, 1.5,
				2.1, 2.2, 2.3, 2.4, 2.5,
				3.1, 3.2, 3.3, 3.4, 3.5,
				4.1, 4.2, 4.3, 4.4, 4.5,
				5.1, 5.2, 5.3, 5.4, 5.5,
			},
			[]int{1, 1, 5, 5},
			"NCHW")

		to.Init(layer.weights,
			[]float64{
				1.1, 1.2, 1.3,
				2.1, 2.2, 2.3,
				3.1, 3.2, 3.3,
			})
		to.Init(layer.bias, []float64{1.0})

		output := layer.Forward(input)

		Expect(output.Descriptor()).To(Equal("NCHW"))
		Expect(output.Size()).To(Equal([]int{1, 1, 3, 3}))

		outputVector := output.Vector()
		expectedOutputVector := []float64{
			20.16, 31.7, 22.68,
			47.54, 72.4, 49.52,
			34.56, 51.5, 34.68,
		}
		for i := range expectedOutputVector {
			Expect(outputVector[i]).
				To(BeNumerically("~", expectedOutputVector[i], 1e-3))
		}
	})

	It("should forward with channel and batch", func() {
		layer = NewConv2D(
			1, to,
			[]int{3, 3, 3}, []int{4, 3, 3, 3}, []int{1, 1}, []int{1, 1})

		input := to.CreateWithData(
			[]float64{
				1.111, 1.112, 1.113, 1.121, 1.122, 1.123, 1.131, 1.132, 1.133,
				1.211, 1.212, 1.213, 1.221, 1.222, 1.223, 1.231, 1.232, 1.233,
				1.311, 1.312, 1.313, 1.321, 1.322, 1.323, 1.331, 1.332, 1.333,
				2.111, 2.112, 2.113, 2.121, 2.122, 2.123, 2.131, 2.132, 2.133,
				2.211, 2.212, 2.213, 2.221, 2.222, 2.223, 2.231, 2.232, 2.233,
				2.311, 2.312, 2.313, 2.321, 2.322, 2.323, 2.331, 2.332, 2.333,
			},
			[]int{2, 3, 3, 3},
			"NCHW")

		to.Init(layer.weights,
			[]float64{
				1.111, 1.112, 1.113, 1.121, 1.122, 1.123, 1.131, 1.132, 1.133,
				1.211, 1.212, 1.213, 1.221, 1.222, 1.223, 1.231, 1.232, 1.233,
				1.311, 1.312, 1.313, 1.321, 1.322, 1.323, 1.331, 1.332, 1.333,
				2.111, 2.112, 2.113, 2.121, 2.122, 2.123, 2.131, 2.132, 2.133,
				2.211, 2.212, 2.213, 2.221, 2.222, 2.223, 2.231, 2.232, 2.233,
				2.111, 2.112, 2.113, 2.121, 2.122, 2.123, 2.131, 2.132, 2.133,
				3.111, 3.112, 3.113, 3.121, 3.122, 3.123, 3.131, 3.132, 3.133,
				3.211, 3.212, 3.213, 3.221, 3.222, 3.223, 3.231, 3.232, 3.233,
				3.311, 3.312, 3.313, 3.321, 3.322, 3.323, 3.331, 3.332, 3.333,
				4.111, 4.112, 4.113, 4.121, 4.122, 4.123, 4.131, 4.132, 4.133,
				4.211, 4.212, 4.213, 4.221, 4.222, 4.223, 4.231, 4.232, 4.233,
				4.111, 4.112, 4.113, 4.121, 4.122, 4.123, 4.131, 4.132, 4.133,
			})
		to.Init(layer.bias, []float64{1.0, 2.0, 3.0, 4.0})

		output := layer.Forward(input)

		Expect(output.Descriptor()).To(Equal("NCHW"))
		Expect(output.Size()).To(Equal([]int{2, 4, 3, 3}))

		outputVector := output.Vector()
		expectedOutputVector := []float64{
			18.999348, 27.999124, 18.999468,
			28.000312, 41.500486, 28.000312,
			18.999468, 27.999124, 18.999348,

			33.544148, 49.324724, 33.555468,
			49.401512, 73.114886, 49.418312,
			33.656268, 49.492724, 33.667348,

			50.195348, 73.811124, 50.219468,
			73.974312, 109.488486, 74.010312,
			50.435468, 74.171124, 50.459348,

			64.740148, 95.136724, 64.775468,
			95.375512, 141.102886, 95.428312,
			65.092268, 95.664724, 65.127348,

			33.729348, 50.085124, 33.717468,
			50.005312, 74.494486, 49.987312,
			33.609468, 49.905124, 33.597348,

			59.474148, 88.210724, 59.473468,
			88.206512, 131.308886, 88.205312,
			59.466268, 88.198724, 59.465348,

			88.925348, 131.897124, 88.937468,
			131.979312, 196.482486, 131.997312,
			89.045468, 132.077124, 89.057348,

			114.670148, 170.022724, 114.693468,
			170.180512, 253.296886, 170.215312,
			114.902268, 170.370724, 114.925348,
		}
		for i := range expectedOutputVector {
			Expect(outputVector[i]).
				To(BeNumerically("~", expectedOutputVector[i], 1e-3))
		}
	})

	It("should do backward propagation", func() {
		layer = NewConv2D(
			1,
			to,
			[]int{1, 3, 3},
			[]int{1, 1, 3, 3},
			[]int{1, 1},
			[]int{1, 1},
		)

		to.Init(layer.weights, []float64{
			1.1, 1.2, 1.3,
			2.1, 2.2, 2.3,
			3.1, 3.2, 3.3})

		layer.forwardInput = to.CreateWithData([]float64{
			1.1, 1.2, 1.3,
			2.1, 2.2, 2.3,
			3.1, 3.2, 3.3,
		}, []int{1, 1, 3, 3}, "NCHW")

		backwardInput := to.CreateWithData([]float64{
			1.1, 1.2, 1.3,
			2.1, 2.2, 2.3,
			3.1, 3.2, 3.3,
		}, []int{1, 1, 3, 3}, "NCHW")

		backwardOutput := layer.Backward(backwardInput)

		expectedWeightGradient := to.CreateWithData(
			[]float64{
				19.160000, 29.080000, 19.560001,
				33.040001, 49.620003, 33.040001,
				19.560001, 29.080000, 19.160000,
			}, []int{9}, "")
		expectedBiasGradient := to.CreateWithData([]float64{19.8}, []int{1}, "")
		expectedInputGradient := to.CreateWithData(
			[]float64{
				9.88, 15.8, 11.24,
				23.72, 37.5, 26.36,
				27.08, 42.2, 29.24,
			}, []int{1, 1, 3, 3}, "")

		tensorMatch(layer.weightGradients, expectedWeightGradient)
		tensorMatch(layer.biasGradients, expectedBiasGradient)
		tensorMatch(backwardOutput, expectedInputGradient)
	})

	It("should do backward propagation, consider stride", func() {
		layer = NewConv2D(
			1,
			to,
			[]int{1, 4, 4},
			[]int{1, 1, 2, 2},
			[]int{2, 2},
			[]int{1, 1},
		)

		to.Init(layer.weights, []float64{
			1.1, 1.2,
			2.1, 2.2,
		})

		layer.forwardInput = to.CreateWithData([]float64{
			1.1, 1.2, 1.3, 1.4,
			2.1, 2.2, 2.3, 2.4,
			3.1, 3.2, 3.3, 3.4,
			4.1, 4.2, 4.3, 4.4,
		}, []int{1, 1, 4, 4}, "NCHW")

		backwardInput := to.CreateWithData([]float64{
			1.1, 1.2, 1.3,
			2.1, 2.2, 2.3,
			3.1, 3.2, 3.3,
		}, []int{1, 1, 3, 3}, "NCHW")

		backwardOutput := layer.Backward(backwardInput)

		expectedWeightGradient := to.CreateWithData(
			[]float64{
				38.32, 35.94,
				18.12, 16.54,
			}, []int{4}, "")
		expectedBiasGradient := to.CreateWithData([]float64{19.8}, []int{1}, "")
		expectedInputGradient := to.CreateWithData(
			[]float64{
				2.42, 2.52, 2.64, 2.73,
				2.52, 2.42, 2.64, 2.53,
				4.62, 4.62, 4.84, 4.83,
				3.72, 3.52, 3.84, 3.63,
			}, []int{1, 1, 4, 4}, "")

		tensorMatch(layer.weightGradients, expectedWeightGradient)
		tensorMatch(layer.biasGradients, expectedBiasGradient)
		tensorMatch(backwardOutput, expectedInputGradient)
	})

	It("should do backward propagation, consider batch and channe", func() {
		layer = NewConv2D(
			1,
			to,
			[]int{3, 3, 3},
			[]int{4, 3, 3, 3},
			[]int{1, 1},
			[]int{1, 1},
		)

		to.Init(layer.weights, []float64{
			1.111, 1.112, 1.113,
			1.121, 1.122, 1.123,
			1.131, 1.132, 1.133,

			1.211, 1.212, 1.213,
			1.221, 1.222, 1.223,
			1.231, 1.232, 1.233,

			1.311, 1.312, 1.313,
			1.321, 1.322, 1.323,
			1.331, 1.332, 1.333,

			2.111, 2.112, 2.113,
			2.121, 2.122, 2.123,
			2.131, 2.132, 2.133,

			2.211, 2.212, 2.213,
			2.221, 2.222, 2.223,
			2.231, 2.232, 2.233,

			2.311, 2.312, 2.313,
			2.321, 2.322, 2.323,
			2.331, 2.332, 2.333,

			3.111, 3.112, 3.113,
			3.121, 3.122, 3.123,
			3.131, 3.132, 3.133,

			3.211, 3.212, 3.213,
			3.221, 3.222, 3.223,
			3.231, 3.232, 3.233,

			3.311, 3.312, 3.313,
			3.321, 3.322, 3.323,
			3.331, 3.332, 3.333,

			4.111, 4.112, 4.113,
			4.121, 4.122, 4.123,
			4.131, 4.132, 4.133,

			4.211, 4.212, 4.213,
			4.221, 4.222, 4.223,
			4.231, 4.232, 4.233,

			4.311, 4.312, 4.313,
			4.321, 4.322, 4.323,
			4.331, 4.332, 4.333,
		})

		layer.forwardInput = to.CreateWithData([]float64{
			1.111, 1.112, 1.113, 1.121, 1.122, 1.123, 1.131, 1.132, 1.133,
			1.211, 1.212, 1.213, 1.221, 1.222, 1.223, 1.231, 1.232, 1.233,
			1.311, 1.312, 1.313, 1.321, 1.322, 1.323, 1.331, 1.332, 1.333,
			2.111, 2.112, 2.113, 2.121, 2.122, 2.123, 2.131, 2.132, 2.133,
			2.211, 2.212, 2.213, 2.221, 2.222, 2.223, 2.231, 2.232, 2.233,
			2.311, 2.312, 2.313, 2.321, 2.322, 2.323, 2.331, 2.332, 2.333,
		}, []int{2, 3, 3, 3}, "NCHW")

		backwardInput := to.CreateWithData([]float64{
			1.111, 1.112, 1.113, 1.121, 1.122, 1.123, 1.131, 1.132, 1.133,
			1.211, 1.212, 1.213, 1.221, 1.222, 1.223, 1.231, 1.232, 1.233,
			1.311, 1.312, 1.313, 1.321, 1.322, 1.323, 1.331, 1.332, 1.333,
			1.411, 1.412, 1.413, 1.421, 1.422, 1.423, 1.431, 1.432, 1.433,
			2.111, 2.112, 2.113, 2.121, 2.122, 2.123, 2.131, 2.132, 2.133,
			2.211, 2.212, 2.213, 2.221, 2.222, 2.223, 2.231, 2.232, 2.233,
			2.311, 2.312, 2.313, 2.321, 2.322, 2.323, 2.331, 2.332, 2.333,
			2.411, 2.412, 2.413, 2.421, 2.422, 2.423, 2.431, 2.432, 2.433,
		}, []int{2, 4, 3, 3}, "NCHW")

		backwardOutput := layer.Backward(backwardInput)

		expectedWeightGradient := to.CreateWithData(
			[]float64{
				23.047031, 34.570614, 23.047112,
				34.571411, 51.857128, 34.571411,
				23.047112, 34.570614, 23.047031,

				24.349030, 36.523018, 24.348312,
				36.518406, 54.776726, 36.517212,
				24.341110, 36.511013, 24.340233,

				25.651031, 38.475414, 25.649513,
				38.465408, 57.696323, 38.463005,
				25.635113, 38.451416, 25.633434,

				24.340233, 36.511013, 24.341110,
				36.517212, 54.776726, 36.518406,
				24.348312, 36.523018, 24.349030,

				25.722231, 38.583416, 25.722311,
				38.584209, 57.876324, 38.584209,
				25.722311, 38.583416, 25.722231,
				27.104233, 40.655815, 27.103512,
				40.651207, 60.975929, 40.650005,
				27.096312, 40.643814, 27.095432,

				25.633434, 38.451416, 25.635113,
				38.463005, 57.696323, 38.465408,
				25.649513, 38.475414, 25.651031,

				27.095432, 40.643814, 27.096312,
				40.650005, 60.975929, 40.651207,
				27.103512, 40.655815, 27.104233,

				28.557430, 42.836220, 28.557514,
				42.837009, 64.255524, 42.837009,
				28.557514, 42.836220, 28.557430,

				26.926634, 40.391815, 26.929111,
				40.408810, 60.615929, 40.412407,
				26.950712, 40.427814, 26.953032,

				28.468632, 42.704216, 28.470312,
				42.715809, 64.075523, 42.718208,
				28.484711, 42.728218, 28.486233,

				30.010632, 45.016617, 30.011513,
				45.022808, 67.535126, 45.024010,
				30.018711, 45.028614, 30.019432,
			}, []int{108}, "")
		expectedBiasGradient := to.CreateWithData(
			[]float64{29.196, 30.996, 32.796, 34.596}, []int{4}, "")
		expectedInputGradient := to.CreateWithData(
			[]float64{
				55.020352, 82.57712, 55.082496,
				82.996088, 124.5642, 83.089544,
				55.643232, 83.51168, 55.705696,

				57.046752, 85.61792, 57.110496,
				86.047688, 129.1434, 86.143544,
				57.685632, 86.57648, 57.749696,

				59.073152, 88.65872, 59.138496,
				89.099288, 133.7226, 89.197544,
				59.728032, 89.64128, 59.793696,

				96.884352, 145.38512, 96.962496,
				145.912088, 218.9562, 146.029544,
				97.667232, 146.55968, 97.745696,

				100.510752, 150.82592, 100.590496,
				151.363688, 227.1354, 151.483544,
				101.309632, 152.02448, 101.389696,

				104.137152, 156.26672, 104.218496,
				156.815288, 235.3146, 156.937544,
				104.952032, 157.48928, 105.033696,
			}, []int{2, 3, 3, 3}, "")

		tensorMatch(layer.weightGradients, expectedWeightGradient)
		tensorMatch(layer.biasGradients, expectedBiasGradient)
		tensorMatch(backwardOutput, expectedInputGradient)
	})

})
