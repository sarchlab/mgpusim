package tensor

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/platform"
)

var _ = Describe("Operator", func() {
	var (
		gpuDriver *driver.Driver
		ctx       *driver.Context
		to        *GPUOperator
	)

	BeforeEach(func() {
		_, gpuDriver = platform.MakeEmuBuilder().
			// WithISADebugging().
			WithoutProgressBar().
			Build()
		gpuDriver.Run()
		ctx = gpuDriver.Init()

		to = NewGPUOperator(gpuDriver, ctx)
	})

	It("should transpose", func() {
		in := to.CreateWithData(
			[]float64{
				15, 18, 9, 30, 36, 18, 25, 30, 15,
				15, 18, 9, 30, 36, 18, 25, 30, 15,
			},

			[]int{1, 2, 3, 3}, "CNHW")

		outData := []float32{
			15, 18, 9, 30, 36, 18, 25, 30, 15,
			15, 18, 9, 30, 36, 18, 25, 30, 15,
		}

		out := to.Transpose(in, []int{1, 0, 2, 3})

		outV := out.Vector()
		for i := range outData {
			Expect(outV[i]).To(BeNumerically("~", outData[i], 1e-3))
		}
		Expect(out.Descriptor()).To(Equal("NCHW"))
	})

	It("should roate 180", func() {
		in := to.CreateWithData(
			[]float64{
				1, 2, 3, 4,
				5, 6, 7, 8,
				9, 10, 11, 12,
			},
			[]int{1, 1, 3, 4}, "")

		out := to.Rotate180(in)

		Expect(out.Vector()).To(Equal(
			[]float64{
				12, 11, 10, 9,
				8, 7, 6, 5,
				4, 3, 2, 1,
			}))
	})

	It("should roate 180, test 2", func() {
		in := to.CreateWithData(
			[]float64{
				1.111, 1.112, 1.113, 1.114,
				1.121, 1.122, 1.123, 1.124,
				1.131, 1.132, 1.133, 1.134,

				1.211, 1.212, 1.213, 1.214,
				1.221, 1.222, 1.223, 1.224,
				1.231, 1.232, 1.233, 1.234,

				1.311, 1.312, 1.313, 1.314,
				1.321, 1.322, 1.323, 1.324,
				1.331, 1.332, 1.333, 1.334,

				2.111, 2.112, 2.113, 2.114,
				2.121, 2.122, 2.123, 2.124,
				2.131, 2.132, 2.133, 2.134,

				2.211, 2.212, 2.213, 2.214,
				2.221, 2.222, 2.223, 2.224,
				2.231, 2.232, 2.233, 2.234,

				2.311, 2.312, 2.313, 2.314,
				2.321, 2.322, 2.323, 2.324,
				2.331, 2.332, 2.333, 2.334,
			}, []int{2, 3, 3, 4}, "")

		out := to.Rotate180(in)

		goldOut := []float64{
			1.134, 1.133, 1.132, 1.131,
			1.124, 1.123, 1.122, 1.121,
			1.114, 1.113, 1.112, 1.111,

			1.234, 1.233, 1.232, 1.231,
			1.224, 1.223, 1.222, 1.221,
			1.214, 1.213, 1.212, 1.211,

			1.334, 1.333, 1.332, 1.331,
			1.324, 1.323, 1.322, 1.321,
			1.314, 1.313, 1.312, 1.311,

			2.134, 2.133, 2.132, 2.131,
			2.124, 2.123, 2.122, 2.121,
			2.114, 2.113, 2.112, 2.111,

			2.234, 2.233, 2.232, 2.231,
			2.224, 2.223, 2.222, 2.221,
			2.214, 2.213, 2.212, 2.211,

			2.334, 2.333, 2.332, 2.331,
			2.324, 2.323, 2.322, 2.321,
			2.314, 2.313, 2.312, 2.311,
		}

		outV := out.Vector()
		for i := 0; i < len(goldOut); i++ {
			Expect(outV[i]).To(BeNumerically("~", goldOut[i], 1e-3))
		}
	})

	It("should dilate", func() {
		in := to.CreateWithData(
			[]float64{
				1, 2, 3,
				4, 5, 6,
				7, 8, 9,
			},
			[]int{1, 1, 3, 3}, "")

		out := to.Dilate(in, []int{3, 2})

		goldOut := []float64{
			1, 0, 2, 0, 3,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			4, 0, 5, 0, 6,
			0, 0, 0, 0, 0,
			0, 0, 0, 0, 0,
			7, 0, 8, 0, 9,
		}

		outV := out.Vector()
		for i := 0; i < len(goldOut); i++ {
			Expect(outV[i]).To(BeNumerically("~", goldOut[i], 1e-3))
		}

	})

	It("should do im2col", func() {
		input := to.CreateWithData(
			[]float64{
				1, 1, 1,
				2, 2, 2,
				3, 3, 3,
			},
			[]int{1, 1, 3, 3},
			"NCHW",
		)
		kernelSize := []int{3, 3}
		padding := []int{1, 1}
		stride := []int{1, 1}
		dilation := []int{1, 1}

		output := to.Im2Col(input, kernelSize, padding, stride, dilation)

		Expect(output.Size()).To(Equal([]int{9, 9}))
		Expect(output.Vector()).To(Equal([]float64{
			0, 0, 0, 0, 1, 1, 0, 2, 2,
			0, 0, 0, 1, 1, 1, 2, 2, 2,
			0, 0, 0, 1, 1, 0, 2, 2, 0,
			0, 1, 1, 0, 2, 2, 0, 3, 3,
			1, 1, 1, 2, 2, 2, 3, 3, 3,
			1, 1, 0, 2, 2, 0, 3, 3, 0,
			0, 2, 2, 0, 3, 3, 0, 0, 0,
			2, 2, 2, 3, 3, 3, 0, 0, 0,
			2, 2, 0, 3, 3, 0, 0, 0, 0,
		}))
	})

	It("should do im2col dilation", func() {
		input := to.CreateWithData(
			[]float64{
				1.1, 1.2, 1.3,
				2.1, 2.2, 2.3,
				3.1, 3.0, 3.3,
			},
			[]int{1, 1, 3, 3},
			"NCHW",
		)
		kernelSize := []int{3, 3}
		padding := []int{1, 1}
		stride := []int{1, 1}
		dilation := []int{2, 2}

		output := to.Im2Col(input, kernelSize, padding, stride, dilation)
		outputV := output.Vector()
		outputGold := []float64{
			0.0,
			0.0,
			0.0,
			0.0,
			2.2,
			0.0,
			0.0,
			0.0,
			0.0,
		}

		Expect(output.Size()).To(Equal([]int{9, 1}))
		for i := range outputGold {
			Expect(outputV[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should do im2col batch & channel", func() {
		input := to.CreateWithData(
			[]float64{
				1111, 1112, 1113, 1121, 1122, 1123, 1131, 1132, 1133,
				1211, 1212, 1213, 1221, 1222, 1223, 1231, 1232, 1233,
				1311, 1312, 1313, 1321, 1322, 1323, 1331, 1332, 1333,
				2111, 2112, 2113, 2121, 2122, 2123, 2131, 2132, 2133,
				2211, 2212, 2213, 2221, 2222, 2223, 2231, 2232, 2233,
				3311, 3312, 3313, 3321, 3322, 3323, 3331, 3332, 3333,
			},
			[]int{2, 3, 3, 3},
			"NCHW",
		)
		kernelSize := []int{3, 3}
		padding := []int{1, 1}
		stride := []int{1, 1}
		dilation := []int{1, 1}

		output := to.Im2Col(input, kernelSize, padding, stride, dilation)

		Expect(output.Size()).To(Equal([]int{27, 18}))
		Expect(output.Vector()).To(Equal([]float64{
			0, 0, 0, 0, 1111, 1112, 0, 1121, 1122, 0, 0, 0, 0, 2111, 2112, 0, 2121, 2122,
			0, 0, 0, 1111, 1112, 1113, 1121, 1122, 1123, 0, 0, 0, 2111, 2112, 2113, 2121, 2122, 2123,
			0, 0, 0, 1112, 1113, 0, 1122, 1123, 0, 0, 0, 0, 2112, 2113, 0, 2122, 2123, 0,
			0, 1111, 1112, 0, 1121, 1122, 0, 1131, 1132, 0, 2111, 2112, 0, 2121, 2122, 0, 2131, 2132,
			1111, 1112, 1113, 1121, 1122, 1123, 1131, 1132, 1133, 2111, 2112, 2113, 2121, 2122, 2123, 2131, 2132, 2133,
			1112, 1113, 0, 1122, 1123, 0, 1132, 1133, 0, 2112, 2113, 0, 2122, 2123, 0, 2132, 2133, 0,
			0, 1121, 1122, 0, 1131, 1132, 0, 0, 0, 0, 2121, 2122, 0, 2131, 2132, 0, 0, 0,
			1121, 1122, 1123, 1131, 1132, 1133, 0, 0, 0, 2121, 2122, 2123, 2131, 2132, 2133, 0, 0, 0,
			1122, 1123, 0, 1132, 1133, 0, 0, 0, 0, 2122, 2123, 0, 2132, 2133, 0, 0, 0, 0,
			0, 0, 0, 0, 1211, 1212, 0, 1221, 1222, 0, 0, 0, 0, 2211, 2212, 0, 2221, 2222,
			0, 0, 0, 1211, 1212, 1213, 1221, 1222, 1223, 0, 0, 0, 2211, 2212, 2213, 2221, 2222, 2223,
			0, 0, 0, 1212, 1213, 0, 1222, 1223, 0, 0, 0, 0, 2212, 2213, 0, 2222, 2223, 0,
			0, 1211, 1212, 0, 1221, 1222, 0, 1231, 1232, 0, 2211, 2212, 0, 2221, 2222, 0, 2231, 2232,
			1211, 1212, 1213, 1221, 1222, 1223, 1231, 1232, 1233, 2211, 2212, 2213, 2221, 2222, 2223, 2231, 2232, 2233,
			1212, 1213, 0, 1222, 1223, 0, 1232, 1233, 0, 2212, 2213, 0, 2222, 2223, 0, 2232, 2233, 0,
			0, 1221, 1222, 0, 1231, 1232, 0, 0, 0, 0, 2221, 2222, 0, 2231, 2232, 0, 0, 0,
			1221, 1222, 1223, 1231, 1232, 1233, 0, 0, 0, 2221, 2222, 2223, 2231, 2232, 2233, 0, 0, 0,
			1222, 1223, 0, 1232, 1233, 0, 0, 0, 0, 2222, 2223, 0, 2232, 2233, 0, 0, 0, 0,
			0, 0, 0, 0, 1311, 1312, 0, 1321, 1322, 0, 0, 0, 0, 3311, 3312, 0, 3321, 3322,
			0, 0, 0, 1311, 1312, 1313, 1321, 1322, 1323, 0, 0, 0, 3311, 3312, 3313, 3321, 3322, 3323,
			0, 0, 0, 1312, 1313, 0, 1322, 1323, 0, 0, 0, 0, 3312, 3313, 0, 3322, 3323, 0,
			0, 1311, 1312, 0, 1321, 1322, 0, 1331, 1332, 0, 3311, 3312, 0, 3321, 3322, 0, 3331, 3332,
			1311, 1312, 1313, 1321, 1322, 1323, 1331, 1332, 1333, 3311, 3312, 3313, 3321, 3322, 3323, 3331, 3332, 3333,
			1312, 1313, 0, 1322, 1323, 0, 1332, 1333, 0, 3312, 3313, 0, 3322, 3323, 0, 3332, 3333, 0,
			0, 1321, 1322, 0, 1331, 1332, 0, 0, 0, 0, 3321, 3322, 0, 3331, 3332, 0, 0, 0,
			1321, 1322, 1323, 1331, 1332, 1333, 0, 0, 0, 3321, 3322, 3323, 3331, 3332, 3333, 0, 0, 0,
			1322, 1323, 0, 1332, 1333, 0, 0, 0, 0, 3322, 3323, 0, 3332, 3333, 0, 0, 0, 0,
		}))
	})

	It("should to sum", func() {
		input := to.CreateWithData(
			[]float64{
				1111, 1112, 1113, 1121, 1122, 1123, 1131, 1132, 1133,
				1211, 1212, 1213, 1221, 1222, 1223, 1231, 1232, 1233,
				1311, 1312, 1313, 1321, 1322, 1323, 1331, 1332, 1333,
				2111, 2112, 2113, 2121, 2122, 2123, 2131, 2132, 2133,
				2211, 2212, 2213, 2221, 2222, 2223, 2231, 2232, 2233,
				2311, 2312, 2313, 2321, 2322, 2323, 2331, 2332, 2333,
			},
			[]int{2, 3, 3, 3},
			"NCHW",
		)

		output := to.Sum(input, []int{0})

		Expect(output.Size()).To(Equal([]int{3, 3, 3}))

		outputGold := []float64{
			3222, 3224, 3226, 3242, 3244, 3246, 3262, 3264, 3266,
			3422, 3424, 3426, 3442, 3444, 3446, 3462, 3464, 3466,
			3622, 3624, 3626, 3642, 3644, 3646, 3662, 3664, 3666,
		}
		outputVector := output.Vector()
		for i := range outputGold {
			Expect(outputVector[i]).To(Equal(outputGold[i]))
		}
	})

	It("should to sum 2", func() {
		input := to.CreateWithData(
			[]float64{
				1111, 1112, 1113, 1121, 1122, 1123, 1131, 1132, 1133,
				1211, 1212, 1213, 1221, 1222, 1223, 1231, 1232, 1233,
				1311, 1312, 1313, 1321, 1322, 1323, 1331, 1332, 1333,
				2111, 2112, 2113, 2121, 2122, 2123, 2131, 2132, 2133,
				2211, 2212, 2213, 2221, 2222, 2223, 2231, 2232, 2233,
				2311, 2312, 2313, 2321, 2322, 2323, 2331, 2332, 2333,
			},
			[]int{2, 3, 3, 3},
			"NCHW",
		)

		output := to.Sum(input, []int{0, 3})

		Expect(output.Size()).To(Equal([]int{3, 3}))

		outputGold := []float64{
			9672, 9732, 9792,
			10272, 10332, 10392,
			10872, 10932, 10992,
		}
		outputVector := output.Vector()
		for i := range outputGold {
			Expect(outputVector[i]).To(Equal(outputGold[i]))
		}
	})

	It("should do softmax", func() {
		input := to.CreateWithData(
			[]float64{1, 2, 3, 4, 1, 2, 3},
			[]int{1, 7}, "",
		)

		output := to.Softmax(input)

		Expect(output.Size()).To(Equal([]int{1, 7}))
		outputGold := []float64{
			0.02364054, 0.06426166, 0.1746813, 0.474833,
			0.02364054, 0.06426166, 0.1746813}
		outputVector := output.Vector()
		for i := range outputGold {
			Expect(outputVector[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should do multi-image", func() {
		input := to.CreateWithData(
			[]float64{
				1, 2, 3, 4, 1, 2, 3,
				1, 2, 3, 4, 1, 2, 3,
			}, []int{2, 7}, "")

		output := to.Softmax(input)

		Expect(output.Size()).To(Equal([]int{2, 7}))
		outputGold := []float64{
			0.02364054, 0.06426166, 0.1746813, 0.474833,
			0.02364054, 0.06426166, 0.1746813,

			0.02364054, 0.06426166, 0.1746813, 0.474833,
			0.02364054, 0.06426166, 0.1746813,
		}
		outputVector := output.Vector()
		for i := range outputGold {
			Expect(outputVector[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should run scaleAdd", func() {
		a := to.CreateWithData([]float64{
			1, 2, 3, 4, 5, 6, 7, 8,
		}, []int{8}, "")
		b := to.CreateWithData([]float64{
			-1, -2, -3, -4, -5, -6, -7, -8,
		}, []int{8}, "")

		output := to.ScaleAdd(1, 1.5, a, b)

		Expect(output.Size()).To(Equal([]int{8}))
		outputGold := []float64{
			-0.5, -1.0, -1.5, -2.0, -2.5, -3.0, -3.5, -4.0,
		}
		outputVector := output.Vector()
		for i := range outputGold {
			Expect(outputVector[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should run element-wise mul", func() {
		a := to.CreateWithData([]float64{
			1, 2, 3, 4, 5, 6, 7, 8,
		}, []int{8}, "")
		b := to.CreateWithData([]float64{
			-1, -2, -3, -4, -5, -6, -7, -8,
		}, []int{8}, "")

		output := to.ElementWiseMul(a, b)

		Expect(output.Size()).To(Equal([]int{8}))
		outputGold := []float64{
			-1, -4, -9, -16, -25, -36, -49, -64,
		}
		outputVector := output.Vector()
		for i := range outputGold {
			Expect(outputVector[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should run RMSProp", func() {
		params := to.CreateWithData([]float64{1, 2, 3, 4}, []int{4}, "")
		gradients := to.CreateWithData([]float64{-1, -2, -3, -4}, []int{4}, "")
		sHistory := to.CreateWithData([]float64{5, 6, 7, 8}, []int{4}, "")

		to.RMSProp(params, gradients, sHistory, 0.2, 0.3)

		sHistoryGold := []float64{1.8, 4.4, 8.6, 14.4}
		paramsGold := []float64{1.224, 2.286, 3.307, 4.316}

		sHistoryV := sHistory.Vector()
		for i := range sHistoryGold {
			Expect(sHistoryV[i]).To(BeNumerically("~", sHistoryGold[i], 1e-3))
		}

		paramsV := params.Vector()
		for i := range paramsGold {
			Expect(paramsV[i]).To(BeNumerically("~", paramsGold[i], 1e-3))
		}
	})

	It("should run adam", func() {
		params := to.CreateWithData([]float64{1, 2, 3, 4}, []int{4}, "")
		gradients := to.CreateWithData([]float64{-1, -2, -3, -4}, []int{4}, "")
		vHistory := to.CreateWithData([]float64{9, 10, 11, 12}, []int{4}, "")
		sHistory := to.CreateWithData([]float64{5, 6, 7, 8}, []int{4}, "")

		to.Adam(params, gradients, vHistory, sHistory, 0.1, 0.2, 0.3)

		vHistoryGold := []float64{0, -0.8, -1.6, -2.4}
		sHistoryGold := []float64{1.8, 4.4, 8.6, 14.4}
		paramsGold := []float64{1, 2.114, 3.164, 4.189}

		vHistoryV := vHistory.Vector()
		for i := range vHistoryGold {
			Expect(vHistoryV[i]).To(BeNumerically("~", vHistoryGold[i], 1e-3))
		}

		sHistoryV := sHistory.Vector()
		for i := range sHistoryGold {
			Expect(sHistoryV[i]).To(BeNumerically("~", sHistoryGold[i], 1e-3))
		}

		paramsV := params.Vector()
		for i := range paramsGold {
			Expect(paramsV[i]).To(BeNumerically("~", paramsGold[i], 1e-3))
		}

	})

	It("should run relu forward", func() {
		in := to.CreateWithData([]float64{2, 3, -4, -6}, []int{4}, "")

		output := to.ReluForward(in)

		outputGold := []float64{2, 3, 0, 0}
		outputV := output.Vector()

		for i := range outputGold {
			Expect(outputV[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should run reluBackward", func() {
		in := to.CreateWithData([]float64{2, 3, 8, -6}, []int{4}, "")
		backin := to.CreateWithData([]float64{-2, 3, 8, 7}, []int{4}, "")
		_ = backin
		output := to.ReluBackward(in, backin)

		outputGold := []float64{-2, 3, 8, 0}
		outputV := output.Vector()

		for i := range outputGold {
			Expect(outputV[i]).To(BeNumerically("~", outputGold[i], 1e-3))
		}
	})

	It("should do maxpooling forward", func() {
		inTensor := to.CreateWithData([]float64{
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
		}, []int{2, 3, 6, 6}, "NCHW")

		out, mask := to.MaxPoolingForward(inTensor,
			[]int{2, 2}, []int{1, 1}, []int{2, 2})

		goldOut := []float64{
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,

			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,

			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,

			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,

			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,

			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
			1, 3, 5, 6,
		}

		goldMask := []int{
			0, 2, 4, 5,
			6, 8, 10, 11,
			18, 20, 22, 23,
			30, 32, 34, 35,

			36, 38, 40, 41,
			42, 44, 46, 47,
			54, 56, 58, 59,
			66, 68, 70, 71,

			72, 74, 76, 77,
			78, 80, 82, 83,
			90, 92, 94, 95,
			102, 104, 106, 107,

			108, 110, 112, 113,
			114, 116, 118, 119,
			126, 128, 130, 131,
			138, 140, 142, 143,

			144, 146, 148, 149,
			150, 152, 154, 155,
			162, 164, 166, 167,
			174, 176, 178, 179,

			180, 182, 184, 185,
			186, 188, 190, 191,
			198, 200, 202, 203,
			210, 212, 214, 215,
		}

		Expect(out.Size()).To(Equal([]int{2, 3, 4, 4}))
		outV := out.Vector()
		for i := range goldOut {
			Expect(outV[i]).To(BeNumerically("~", goldOut[i], 1e-3))
		}

		maskV := make([]int32, mask.NumElement())
		gpuDriver.MemCopyD2H(ctx, maskV, mask.(*Tensor).ptr)

		for i := range goldMask {
			Expect(int(maskV[i])).To(Equal(goldMask[i]))
		}
	})

	It("should do maxpooling backward", func() {
		forwardInTensor := to.CreateWithData([]float64{
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,

			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
			1, 2, 3, 4, 5, 6,
		}, []int{2, 3, 6, 6}, "NCHW")
		backwardInTensor := to.CreateWithData([]float64{
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,

			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,

			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,

			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,

			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,

			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
		}, []int{2, 3, 4, 4}, "NCHW")
		maskTensor := to.Create([]int{2, 3, 4, 4})
		maskRaw := []int32{
			0, 2, 4, 5,
			6, 8, 10, 11,
			18, 20, 22, 23,
			30, 32, 34, 35,

			36, 38, 40, 41,
			42, 44, 46, 47,
			54, 56, 58, 59,
			66, 68, 70, 71,

			72, 74, 76, 77,
			78, 80, 82, 83,
			90, 92, 94, 95,
			102, 104, 106, 107,

			108, 110, 112, 113,
			114, 116, 118, 119,
			126, 128, 130, 131,
			138, 140, 142, 143,

			144, 146, 148, 149,
			150, 152, 154, 155,
			162, 164, 166, 167,
			174, 176, 178, 179,

			180, 182, 184, 185,
			186, 188, 190, 191,
			198, 200, 202, 203,
			210, 212, 214, 215,
		}
		gpuDriver.MemCopyH2D(ctx, maskTensor.(*Tensor).ptr, maskRaw)

		out := to.MaxPoolingBackward(
			forwardInTensor, backwardInTensor, maskTensor,
			[]int{2, 2}, []int{1, 1}, []int{2, 2})

		goldOut := []float64{
			1, 0, 2, 0, 3, 4,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,

			1, 0, 2, 0, 3, 4,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,

			1, 0, 2, 0, 3, 4,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,

			1, 0, 2, 0, 3, 4,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,

			1, 0, 2, 0, 3, 4,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,

			1, 0, 2, 0, 3, 4,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
			0, 0, 0, 0, 0, 0,
			1, 0, 2, 0, 3, 4,
		}

		Expect(out.Size()).To(Equal([]int{2, 3, 6, 6}))
		outV := out.Vector()
		for i := range goldOut {
			Expect(outV[i]).To(BeNumerically("~", goldOut[i], 1e-3))
		}
	})

	It("should not do gemm if not matrix", func() {
		matrixA := to.Create([]int{1, 1, 2})

		Expect(func() {
			to.Gemm(false, false, 2.0, 3.0, matrixA, matrixA, matrixA)
		}).To(Panic())
	})

	It("should not do gemm if A, B size mismatch", func() {
		matrixA := to.Create([]int{2, 3})
		matrixB := to.Create([]int{4, 2})

		Expect(func() {
			to.Gemm(false, false, 2.0, 3.0, matrixA, matrixB, matrixA)
		}).To(Panic())
	})

	It("should not do gemm if C size mismatch", func() {
		matrixA := to.Create([]int{2, 3})
		matrixB := to.Create([]int{3, 5})
		matrixC := to.Create([]int{2, 6})

		Expect(func() {
			to.Gemm(false, false, 2.0, 3.0, matrixA, matrixB, matrixC)
		}).To(Panic())
	})

	It("should do gemm", func() {
		matrixA := to.CreateWithData([]float64{
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
			1, 2, 3, 4,
		}, []int{4, 4}, "")
		matrixB := to.CreateWithData([]float64{
			8, 7, 6, 5,
			8, 7, 6, 5,
			8, 7, 6, 5,
			8, 7, 6, 5,
		}, []int{4, 4}, "")
		matrixC := to.CreateWithData([]float64{
			1, 1, 1, 1,
			1, 1, 1, 1,
			1, 1, 1, 1,
			1, 1, 1, 1,
		}, []int{4, 4}, "")

		out := to.Gemm(false, false, 2.0, 3.0, matrixA, matrixB, matrixC)

		outGold := []float64{
			163, 143, 123, 103,
			163, 143, 123, 103,
			163, 143, 123, 103,
			163, 143, 123, 103,
		}

		Expect(out.Size()).To(Equal([]int{4, 4}))
		outV := out.Vector()
		for i := range outGold {
			Expect(outV[i]).To(BeNumerically("~", outGold[i], 1e-3))
		}
	})

	It("should do gemm non-square", func() {
		matrixA := to.CreateWithData([]float64{
			0.000000, 0.000000, 0.030425, 0.627280,
			0.000000, 0.000000, 0.000000, 0.283799,
			0.000000, 0.042333, 0.194985, 0.564994,
			0.000000, 0.229156, 0.000000, 0.221513,
		}, []int{4, 4}, "")
		matrixB := to.CreateWithData([]float64{
			-0.142868, -0.059671,
			-0.090971, -0.015555,
			-0.108483, -0.103449,
			0.089542, -0.140723,
		}, []int{4, 2}, "")
		matrixC := to.CreateWithData([]float64{
			-0.593626, -0.278257,
			-0.593626, -0.278257,
			-0.593626, -0.278257,
			-0.593626, -0.278257,
		}, []int{4, 2}, "")

		out := to.Gemm(false, false, 1.0, 1.0, matrixA, matrixB, matrixC)

		outGold := []float64{
			-0.540759, -0.369678,
			-0.568214, -0.318194,
			-0.568039, -0.378595,
			-0.594638, -0.312994,
		}

		Expect(out.Size()).To(Equal([]int{4, 2}))
		outV := out.Vector()

		fmt.Println(outV)
		for i := range outGold {
			Expect(outV[i]).To(BeNumerically("~", outGold[i], 1e-3))
		}
	})

	It("should do gemm with transpose", func() {
		matrixA := to.CreateWithData([]float64{
			1, 1, 1, 1,
			2, 2, 2, 2,
			3, 3, 3, 3,
			4, 4, 4, 4,
		}, []int{4, 4}, "")
		matrixB := to.CreateWithData([]float64{
			8, 7, 6, 5,
			8, 7, 6, 5,
			8, 7, 6, 5,
			8, 7, 6, 5,
		}, []int{4, 4}, "")
		matrixC := to.CreateWithData([]float64{
			1, 1, 1, 1,
			1, 1, 1, 1,
			1, 1, 1, 1,
			1, 1, 1, 1,
		}, []int{4, 4}, "")

		out := to.Gemm(true, false, 2.0, 3.0, matrixA, matrixB, matrixC)

		outGold := []float64{
			163, 143, 123, 103,
			163, 143, 123, 103,
			163, 143, 123, 103,
			163, 143, 123, 103,
		}

		Expect(out.Size()).To(Equal([]int{4, 4}))
		outV := out.Vector()
		for i := range outGold {
			Expect(outV[i]).To(BeNumerically("~", outGold[i], 1e-3))
		}
	})

	It("should calculate softmax cross entropy derivative", func() {
		input := to.CreateWithData([]float64{
			0.1, 0.2, 0.3, 0.4,
			0.5, 0.6, 0.7, 0.8,
		}, []int{2, 4}, "")
		label := []int{1, 2}

		output := to.SoftmaxCrossEntropyDerivative(input, label)

		outGold := []float64{
			0.1, -0.8, 0.3, 0.4,
			0.5, 0.6, -0.3, 0.8,
		}
		outV := output.Vector()

		Expect(output.Size()).To(Equal([]int{2, 4}))
		for i := range outGold {
			Expect(outV[i]).To(BeNumerically("~", outGold[i], 1e-3))
		}
	})
})
