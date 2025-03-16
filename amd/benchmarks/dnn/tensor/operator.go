package tensor

import (
	"fmt"
	"math"
	"sort"

	"gonum.org/v1/gonum/blas"
	"gonum.org/v1/gonum/blas/blas64"
	"gonum.org/v1/gonum/mat"
)

// Operator can process tensors
type Operator interface {
	Create(size []int) Tensor
	CreateWithData(data []float64, size []int, descriptor string) Tensor
	Free(t Tensor)
	Copy(dst, src Tensor)
	Clone(t Tensor) Tensor
	Dump(t Tensor) string

	Init(t Tensor, data []float64)
	Slice(t Tensor, start, end int) Tensor
	Repeat(t Tensor, times int) Tensor
	Clear(t Tensor)
	Zeros(size []int) Tensor

	Reshape(t Tensor, newSize []int) Tensor
	Transpose(t Tensor, order []int) Tensor
	Rotate180(t Tensor) Tensor
	Dilate(t Tensor, dilate []int) Tensor

	Sum(t Tensor, axis []int) Tensor
	Gemm(transA, transB bool, alpha, beta float64, a, b, c Tensor) Tensor
	Im2Col(t Tensor, kernelSize, padding, stride, dilation []int) Tensor
	MaxPoolingForward(t Tensor,
		kernelSize, padding, stride []int,
	) (out, mask Tensor)
	MaxPoolingBackward(
		forwardIn, backwardIn, mask Tensor,
		kernelSize, padding, stride []int) Tensor
	AvgPoolingForward(t Tensor,
		kernelSize, padding, stride []int) Tensor
	AvgPoolingBackward(forwardIn, backwardIn Tensor,
		kernelSize, padding, stride []int) Tensor
	Softmax(t Tensor) Tensor
	CrossEntropy(t Tensor, label []int) float64
	CrossEntropyDerivative(t Tensor, label []int) Tensor
	SoftmaxCrossEntropyDerivative(t Tensor, label []int) Tensor

	ElementWiseMul(t1, t2 Tensor) Tensor
	ScaleAdd(alpha, beta float64, a, b Tensor) Tensor

	RMSProp(params, gradient, sHistory Tensor,
		smoothFactor, learningRate float64)
	Adam(params, gradients, vHistory, sHistory Tensor,
		smoothFactor1, smoothFactor2, learningRate float64)

	ReluForward(in Tensor) Tensor
	ReluBackward(forwardIn, backwardIn Tensor) Tensor
}

// CPUOperator can process CPU tensors.
type CPUOperator struct {
}

// Create creates a new CPU tensor.
func (to CPUOperator) Create(size []int) Tensor {
	t := &SimpleTensor{
		data: make([]float64, numElement(size)),
	}

	t.size = make([]int, len(size))
	copy(t.size, size)

	return t
}

// CreateWithData creates a new CPU tensor and fill it with the given data.
func (to CPUOperator) CreateWithData(
	data []float64,
	size []int,
	descriptor string,
) Tensor {
	t := &SimpleTensor{
		descriptor: descriptor,
		data:       make([]float64, numElement(size)),
	}

	t.size = make([]int, len(size))
	copy(t.size, size)

	to.Init(t, data)

	return t
}

// Free does nothing as Go's gabage collector will take care of it.
func (to CPUOperator) Free(t Tensor) {
	// Do nothing
}

// Copy moves the data, size, and descriptor from the src tensor to the
// destination tensor.
func (to CPUOperator) Copy(dst, src Tensor) {
	d := dst.(*SimpleTensor)
	s := src.(*SimpleTensor)

	if len(d.data) != len(s.data) {
		panic("mismatch in size")
	}

	copy(d.data, s.data)
}

// Dump converts the content of the tensor to a string.
func (to CPUOperator) Dump(t Tensor) string {
	v := t.Vector()

	dimSize := make([]int, len(t.Size()))
	product := 1
	for i := len(t.Size()) - 1; i >= 0; i-- {
		product *= t.Size()[i]
		dimSize[i] = product
	}

	out := ""
	indent := 0
	for i := 0; i < t.NumElement(); i++ {
		for _, d := range dimSize {
			if i%d == 0 {
				out += "\n"
				for k := 0; k < indent; k++ {
					out += " "
				}
				out += "["
				indent++
			}
		}

		out += fmt.Sprintf("%4f, ", v[i])

		for _, d := range dimSize {
			if (i+1)%d == 0 {
				out += "],"
				indent--
			}
		}
	}
	out += "\n"

	return out
}

// Init initialize tensor with data and size.
func (to CPUOperator) Init(t Tensor, data []float64) {
	cpuTensor := t.(*SimpleTensor)

	if numElement(t.Size()) != len(data) {
		panic("mismatch in buffer shape")
	}

	if len(cpuTensor.data) != numElement(t.Size()) {
		panic("buffer size not enough")
	}

	copy(cpuTensor.data, data)
}

// Clear sets all the element to 0.
func (to CPUOperator) Clear(t Tensor) {
	d := t.(*SimpleTensor).data
	for i := range d {
		d[i] = 0
	}
}

// Zeros creates a tensor with given size and fill the tensor with 0s.
func (to CPUOperator) Zeros(size []int) Tensor {
	return to.Create(size)
}

// Slice creates another tensor that shares part of the underlying buffer
// bounded by [start, end).
func (to CPUOperator) Slice(
	t Tensor,
	start, end int,
) Tensor {
	out := &SimpleTensor{
		size: []int{end - start},
		data: t.(*SimpleTensor).data[start:end],
	}

	return out
}

// Repeat will create a new tensor that duplicates the input tensor by n times.
func (to CPUOperator) Repeat(t Tensor, times int) Tensor {
	numElem := numElement(t.Size())
	out := to.Create([]int{numElem * times}).(*SimpleTensor)
	inData := t.Vector()

	for i := 0; i < times; i++ {
		copy(out.data[i*numElem:(i+1)*numElem], inData)
	}

	return out
}

// Clone duplicates the tensor.
func (to CPUOperator) Clone(t Tensor) Tensor {
	inT := t.(*SimpleTensor)
	outT := &SimpleTensor{descriptor: inT.descriptor}

	outT.size = make([]int, len(inT.size))
	copy(outT.size, inT.size)

	outT.data = make([]float64, numElement(outT.size))

	to.Init(outT, inT.data)

	return outT
}

// Reshape creates a new tensor wit the same element but a different shape.
func (to CPUOperator) Reshape(t Tensor, newSize []int) Tensor {
	out := to.Clone(t).(*SimpleTensor)

	out.size = make([]int, len(newSize))
	copy(out.size, newSize)

	to.numElementMustMatch(t, out)

	return out
}

// Rotate180 rotates all the lowest level matrics by 180 degree.
func (to CPUOperator) Rotate180(t Tensor) Tensor {
	in := t.(*SimpleTensor)
	inV := in.data

	inSize := in.size
	outSize := in.size

	out := to.Create(outSize).(*SimpleTensor)
	outV := out.data
	for i := 0; i < len(inV); i++ {
		outIndex := i

		outPos := to.unflatIndex(outIndex, outSize)
		inPos := make([]int, len(outPos))
		copy(inPos, outPos)

		inPos[len(inPos)-1] = inSize[len(inPos)-1] - outPos[len(inPos)-1] - 1
		inPos[len(inPos)-2] = inSize[len(inPos)-2] - outPos[len(inPos)-2] - 1

		inIndex := to.flatIndex(inPos, inSize)

		outV[outIndex] = inV[inIndex]
	}

	return out
}

// Dilate add 0s in the rows and columns.
func (to CPUOperator) Dilate(t Tensor, dilate []int) Tensor {
	in := t.(*SimpleTensor)

	outSize := make([]int, len(in.size))
	copy(outSize, in.size)

	outSize[len(outSize)-1] = (outSize[len(outSize)-1]-1)*dilate[1] + 1
	outSize[len(outSize)-2] = (outSize[len(outSize)-2]-1)*dilate[0] + 1

	out := to.Create(outSize).(*SimpleTensor)

	for i := 0; i < len(out.data); i++ {
		outPos := to.unflatIndex(i, out.size)

		outValue := float64(0)
		if outPos[len(outPos)-1]%dilate[1] == 0 &&
			outPos[len(outPos)-2]%dilate[0] == 0 {
			inPos := make([]int, len(outPos))
			copy(inPos, outPos)

			inPos[len(inPos)-1] /= dilate[1]
			inPos[len(inPos)-2] /= dilate[0]

			inIndex := to.flatIndex(inPos, t.Size())

			outValue = in.data[inIndex]
		}

		out.data[i] = outValue
	}

	return out
}

// Transpose can reorder the axes.
func (to CPUOperator) Transpose(t Tensor, order []int) Tensor {
	if len(order) != len(t.Size()) {
		panic("order should include all indices")
	}

	inputSize := t.Size()
	outputSize := make([]int, len(t.Size()))

	for i := 0; i < len(order); i++ {
		outputSize[i] = inputSize[order[i]]
	}

	output := to.Create(outputSize).(*SimpleTensor)
	outputData := output.data
	inputData := t.(*SimpleTensor).data

	for i := 0; i < len(outputData); i++ {
		outputIndex := to.unflatIndex(i, outputSize)
		inputIndex := make([]int, len(outputIndex))
		for j := 0; j < len(inputIndex); j++ {
			inputIndex[order[j]] = outputIndex[j]
		}

		inputIndexFlat := to.flatIndex(inputIndex, inputSize)

		outputData[i] = inputData[inputIndexFlat]
	}

	output.descriptor = ""
	for i := 0; i < len(t.Descriptor()); i++ {
		output.descriptor += string(t.Descriptor()[order[i]])
	}

	return output
}

// Sum calculate sums over given axis.
func (to CPUOperator) Sum(t Tensor, axis []int) Tensor {
	sort.Ints(axis)

	curr := to.Clone(t).(*SimpleTensor)
	var next *SimpleTensor
	for i, a := range axis {
		next = to.sumOneAxis(curr, a-i)
		to.Free(curr)
		curr = next
	}

	return curr
}

func (to CPUOperator) sumOneAxis(t *SimpleTensor, axis int) *SimpleTensor {
	outSize := []int{}
	for i := 0; i < len(t.size); i++ {
		if i != axis {
			outSize = append(outSize, t.size[i])
		}
	}

	out := to.Zeros(outSize).(*SimpleTensor)
	inData := t.data
	outData := out.data
	numElem := numElement(outSize)

	for i := 0; i < numElem; i++ {
		outIndex := to.unflatIndex(i, outSize)
		for j := 0; j < t.size[axis]; j++ {
			inIndex := to.sumOutIndexToInIndex(outIndex, t.size, j, axis)

			inFlatIndex := to.flatIndex(inIndex, t.size)

			outData[i] += inData[inFlatIndex]
		}
	}

	return out
}

func (to CPUOperator) sumOutIndexToInIndex(
	outIndex, inputSize []int,
	axisIndex, axis int,
) []int {
	inIndex := make([]int, len(inputSize))

	axisIndexAdded := false
	for k := 0; k < len(inIndex); k++ {
		if k == axis {
			inIndex[k] = axisIndex
			axisIndexAdded = true
		} else if !axisIndexAdded {
			inIndex[k] = outIndex[k]
		} else {
			inIndex[k] = outIndex[k-1]
		}
	}

	return inIndex
}

func (to CPUOperator) unflatIndex(flatIndex int, size []int) []int {
	out := make([]int, len(size))

	accumulatedSize := numElement(size)
	for i := 0; i < len(out); i++ {
		accumulatedSize /= size[i]
		out[i] = flatIndex / accumulatedSize
		flatIndex -= out[i] * accumulatedSize
	}

	return out
}

func (to CPUOperator) flatIndex(index, size []int) int {
	out := 0
	accumulatedSize := 1

	for i := len(size) - 1; i >= 0; i-- {
		out += index[i] * accumulatedSize
		accumulatedSize *= size[i]
	}

	return out
}

// Gemm performs alpha x A x B + beta x C operation.
func (to CPUOperator) Gemm(
	transA, transB bool,
	alpha, beta float64,
	a, b, c Tensor,
) Tensor {
	to.mustBeTwoDimension(a)
	to.mustBeTwoDimension(b)
	to.mustBeTwoDimension(c)

	out := to.Clone(c)

	ma := mat.NewDense(a.Size()[0], a.Size()[1], a.Vector())
	mb := mat.NewDense(b.Size()[0], b.Size()[1], b.Vector())
	mc := mat.NewDense(c.Size()[0], c.Size()[1], out.Vector())

	gemmTransA := blas.NoTrans
	if transA {
		gemmTransA = blas.Trans
	}

	gemmTransB := blas.NoTrans
	if transB {
		gemmTransB = blas.Trans
	}

	blas64.Gemm(gemmTransA, gemmTransB,
		1, ma.RawMatrix(), mb.RawMatrix(), 1, mc.RawMatrix())

	return out
}

// Im2Col performs the im2col operation.
func (to CPUOperator) Im2Col(
	t Tensor,
	kernelSize, padding, stride, dilation []int,
) Tensor {
	inputSize := t.Size()
	inputData := t.(*SimpleTensor).data

	batch := inputSize[0]
	channel := inputSize[1]
	width := inputSize[2]
	height := inputSize[3]

	kernelHeight := kernelSize[0]
	kernelWidth := kernelSize[1]

	effKernelHeight := (kernelSize[0]-1)*dilation[0] + 1
	effKernelWidth := (kernelSize[1]-1)*dilation[1] + 1

	fieldHeight := (height-effKernelHeight+2*padding[0])/stride[0] + 1
	fieldWidth := (width-effKernelWidth+2*padding[1])/stride[1] + 1

	outWidth := fieldHeight * fieldWidth * batch
	outHeight := kernelHeight * kernelWidth * channel

	output := to.Create([]int{outHeight, outWidth})
	outputData := output.(*SimpleTensor).data

	frameSize := width * height
	pictureSize := channel * frameSize

	for i := int(0); i < batch*fieldWidth*fieldHeight; i++ {
		batchID := i / (fieldWidth * fieldHeight)
		blockID := i % (fieldWidth * fieldHeight)
		blockX := blockID % fieldWidth
		blockY := blockID / fieldWidth

		for j := int(0); j < outHeight; j++ {
			channelID := j / (kernelWidth * kernelHeight)
			y := j % (kernelWidth * kernelHeight) / kernelWidth
			x := j % kernelWidth

			realY := y*dilation[0] + blockY*stride[0] - padding[0]
			realX := x*dilation[1] + blockX*stride[1] - padding[1]

			inputIndex := batchID*pictureSize +
				channelID*frameSize +
				realY*width + realX
			outputIndex := j*outWidth + i

			out := float64(0)
			if realX >= 0 && realY >= 0 && realX < width && realY < height {
				out = inputData[inputIndex]
			}

			outputData[outputIndex] = out
		}
	}

	return output
}

// MaxPoolingForward calculates the forward propagation results for MaxPooling
// layers.
func (to CPUOperator) MaxPoolingForward(
	t Tensor,
	kernelSize, padding, stride []int,
) (out, mask Tensor) {
	in := t.(*SimpleTensor)

	if len(in.size) != 4 {
		panic("maxpooling input must be 4D")
	}

	outSize := make([]int, len(in.size))
	copy(outSize, in.size)

	dilation := []int{1, 1}
	effKernelHeight := (kernelSize[0]-1)*dilation[0] + 1
	effKernelWidth := (kernelSize[1]-1)*dilation[1] + 1
	fieldHeight := (outSize[2]-effKernelHeight+2*padding[0])/stride[0] + 1
	fieldWidth := (outSize[3]-effKernelWidth+2*padding[1])/stride[1] + 1

	outSize[2] = fieldWidth
	outSize[3] = fieldHeight

	out = to.Create(outSize)
	mask = to.Create(outSize)
	outData := out.(*SimpleTensor).data
	maskData := mask.(*SimpleTensor).data
	inData := in.data

	for i := range outData {
		outPos := to.unflatIndex(i, outSize)

		max := -math.MaxFloat64
		maxIndex := 0
		for h := 0; h < kernelSize[0]; h++ {
			for w := 0; w < kernelSize[1]; w++ {
				inPos := make([]int, len(outPos))
				copy(inPos, outPos)

				inPos[2] = outPos[2]*stride[0] + h*dilation[0] - padding[0]
				inPos[3] = outPos[3]*stride[1] + w*dilation[1] - padding[1]

				if inPos[2] < 0 || inPos[3] < 0 || inPos[2] >= in.size[2] || inPos[3] >= in.size[3] {
					continue
				}

				inIndex := to.flatIndex(inPos, in.size)

				inValue := inData[inIndex]
				if inValue > max {
					max = inValue
					maxIndex = inIndex
				}
			}
		}

		outData[i] = max
		maskData[i] = float64(maxIndex)
	}

	return out, mask
}

// MaxPoolingBackward calculates the back propagation results for MaxPooling
// layers.
func (to CPUOperator) MaxPoolingBackward(
	forwardIn, backwardIn, mask Tensor,
	kernelSize, padding, stride []int,
) Tensor {
	fIn := forwardIn.(*SimpleTensor)
	bIn := backwardIn.(*SimpleTensor)
	maskT := mask.(*SimpleTensor)
	out := to.Create(fIn.size).(*SimpleTensor)

	outData := out.data

	for i := 0; i < len(outData); i++ {
		outPos := to.unflatIndex(i, out.size)

		pooledYStart := (outPos[2]+padding[0]-kernelSize[0])/stride[0] + 1
		pooledYEnd := (outPos[2]+padding[0])/stride[0] + 1
		if outPos[2]+padding[0] < kernelSize[0] {
			pooledYStart = 0
		}

		pooledXStart := (outPos[3]+padding[1]-kernelSize[1])/stride[1] + 1
		pooledXEnd := (outPos[3]+padding[1])/stride[1] + 1
		if outPos[3]+padding[1] < kernelSize[1] {
			pooledXStart = 0
		}

		gradient := 0.0
		for py := pooledYStart; py < pooledYEnd; py++ {
			for px := pooledXStart; px < pooledXEnd; px++ {
				if to.isOutside([]int{py, px}, bIn.size[2:4]) {
					continue
				}

				inPos := make([]int, len(bIn.size))
				copy(inPos, outPos)
				inPos[2] = py
				inPos[3] = px
				inIndex := to.flatIndex(inPos, bIn.size)

				if int(maskT.data[inIndex]) == i {
					gradient += bIn.data[inIndex]
				}
			}
		}

		outData[i] = gradient
	}

	return out
}

func (to CPUOperator) isOutside(pos, size []int) bool {
	for i := 0; i < len(pos); i++ {
		if pos[i] < 0 {
			return true
		}

		if pos[i] >= size[i] {
			return true
		}
	}

	return false
}

// AvgPoolingForward calculates the forward propagation results for AvgPooling
// layers.
func (to CPUOperator) AvgPoolingForward(
	t Tensor,
	kernelSize, padding, stride []int,
) Tensor {
	in := t.(*SimpleTensor)

	if len(in.size) != 4 {
		panic("maxpooling input must be 4D")
	}

	outSize := make([]int, len(in.size))
	copy(outSize, in.size)

	dilation := []int{1, 1}
	effKernelHeight := (kernelSize[0]-1)*dilation[0] + 1
	effKernelWidth := (kernelSize[1]-1)*dilation[1] + 1
	fieldHeight := (outSize[2]-effKernelHeight+2*padding[0])/stride[0] + 1
	fieldWidth := (outSize[3]-effKernelWidth+2*padding[1])/stride[1] + 1

	outSize[2] = fieldWidth
	outSize[3] = fieldHeight

	out := to.Create(outSize).(*SimpleTensor)
	outData := out.data
	inData := in.data

	for i := range outData {
		outPos := to.unflatIndex(i, outSize)

		sum := float64(0)
		for h := 0; h < kernelSize[0]; h++ {
			for w := 0; w < kernelSize[1]; w++ {
				inPos := make([]int, len(outPos))
				copy(inPos, outPos)

				inPos[2] = outPos[2]*stride[0] + h*dilation[0] - padding[0]
				inPos[3] = outPos[3]*stride[1] + w*dilation[1] - padding[1]

				if inPos[2] < 0 || inPos[3] < 0 || inPos[2] >= in.size[2] || inPos[3] >= in.size[3] {
					continue
				}

				inIndex := to.flatIndex(inPos, in.size)

				sum += inData[inIndex]
			}
		}

		outData[i] = sum / float64(kernelSize[0]*kernelSize[1])
	}

	return out
}

// AvgPoolingBackward calculates the backward propagation results for AvgPooling
// layers.
func (to CPUOperator) AvgPoolingBackward(forwardIn, backwardIn Tensor,
	kernelSize, padding, stride []int) Tensor {
	fIn := forwardIn.(*SimpleTensor)
	bIn := backwardIn.(*SimpleTensor)
	out := to.Create(fIn.size).(*SimpleTensor)

	outData := out.data

	for i := 0; i < len(outData); i++ {
		outPos := to.unflatIndex(i, out.size)

		pooledYStart := (outPos[2]+padding[0]-kernelSize[0])/stride[0] + 1
		pooledYEnd := (outPos[2]+padding[0])/stride[0] + 1
		if outPos[2]+padding[0] < kernelSize[0] {
			pooledYStart = 0
		}

		pooledXStart := (outPos[3]+padding[1]-kernelSize[1])/stride[1] + 1
		pooledXEnd := (outPos[3]+padding[1])/stride[1] + 1
		if outPos[3]+padding[1] < kernelSize[1] {
			pooledXStart = 0
		}

		gradient := 0.0
		for py := pooledYStart; py < pooledYEnd; py++ {
			for px := pooledXStart; px < pooledXEnd; px++ {
				if to.isOutside([]int{py, px}, bIn.size[2:4]) {
					continue
				}

				inPos := make([]int, len(bIn.size))
				copy(inPos, outPos)
				inPos[2] = py
				inPos[3] = px
				inIndex := to.flatIndex(inPos, bIn.size)

				gradient += bIn.data[inIndex] /
					float64(kernelSize[0]*kernelSize[1])
			}
		}

		outData[i] = gradient
	}

	return out
}

func (to CPUOperator) mustBeTwoDimension(t Tensor) {
	if len(t.Size()) != 2 {
		panic("expecting a matrix")
	}
}

// Softmax caculates the softmax vector.
func (to CPUOperator) Softmax(t Tensor) Tensor {
	to.mustBeTwoDimension(t)

	output := to.Create(t.Size()).(*SimpleTensor)
	outputD := output.data
	inputD := t.(*SimpleTensor).data

	for i := 0; i < t.Size()[0]; i++ {
		start := i * t.Size()[1]
		end := start + t.Size()[1]
		inputSlice := inputD[start:end]
		denominator := to.softmaxDenominator(inputSlice)

		for j := 0; j < t.Size()[1]; j++ {
			index := i*t.Size()[1] + j
			outputD[index] = math.Exp(inputD[index]) / denominator
		}
	}

	return output
}

func (to CPUOperator) softmaxDenominator(array []float64) float64 {
	sum := 0.0

	for _, val := range array {
		sum += math.Exp(val)
	}

	return sum
}

// CrossEntropy calculates the cross entropy.
func (to CPUOperator) CrossEntropy(t Tensor, label []int) float64 {
	to.mustBeTwoDimension(t)

	loss := 0.0
	for i := 0; i < t.Size()[0]; i++ {
		start := i * t.Size()[1]
		end := start + t.Size()[1]
		inputSlice := t.(*SimpleTensor).data[start:end]

		loss += -math.Log(inputSlice[label[i]])
	}

	loss /= float64(t.Size()[0])

	return loss
}

// CrossEntropyDerivative generators the final derivative of the forward pass.
func (to CPUOperator) CrossEntropyDerivative(t Tensor, label []int) Tensor {
	to.mustBeTwoDimension(t)

	input := t.(*SimpleTensor)
	output := to.Create(t.Size()).(*SimpleTensor)

	for i := 0; i < input.size[0]; i++ {
		for j := 0; j < input.size[1]; j++ {
			index := i*output.size[1] + j
			if label[i] == j {
				output.data[index] = -1 / input.data[index]
			}
		}
	}

	return output
}

// SoftmaxCrossEntropyDerivative generators the final derivative of the forward
// pass.
func (to CPUOperator) SoftmaxCrossEntropyDerivative(
	t Tensor,
	label []int,
) Tensor {
	to.mustBeTwoDimension(t)

	input := t.(*SimpleTensor)
	output := to.Create(t.Size()).(*SimpleTensor)

	for i := 0; i < input.size[0]; i++ {
		for j := 0; j < input.size[1]; j++ {
			index := i*output.size[1] + j
			if label[i] == j {
				output.data[index] = input.data[index] - 1
			} else {
				output.data[index] = input.data[index]
			}
		}
	}

	return output
}

// ElementWiseMul performs element-wise multiplication operation.
func (to CPUOperator) ElementWiseMul(t1, t2 Tensor) Tensor {
	to.numElementMustMatch(t1, t2)

	ct1 := t1.(*SimpleTensor)
	ct2 := t2.(*SimpleTensor)
	outT := to.Create(t1.Size()).(*SimpleTensor)

	for i := 0; i < len(ct1.data); i++ {
		outT.data[i] = ct1.data[i] * ct2.data[i]
	}

	return outT
}

// ScaleAdd performs the alpht*A + beta*B operation
func (to CPUOperator) ScaleAdd(alpha, beta float64, a, b Tensor) Tensor {
	ca := a.(*SimpleTensor).data
	cb := b.(*SimpleTensor).data

	out := to.Create(a.Size()).(*SimpleTensor)

	for i := 0; i < len(ca); i++ {
		out.data[i] = alpha*ca[i] + beta*cb[i]
	}

	return out
}

// RMSProp runs the rmsProp gradient descent algorithm.
func (to CPUOperator) RMSProp(
	params, gradients Tensor,
	sHistory Tensor,
	smoothFactor, learningRate float64,
) {
	p := params.(*SimpleTensor).data
	g := gradients.(*SimpleTensor).data
	s := sHistory.(*SimpleTensor).data

	for i := 0; i < len(p); i++ {
		s[i] = smoothFactor*s[i] + (1-smoothFactor)*g[i]*g[i]
		p[i] -= learningRate * (1.0 / (math.Sqrt(s[i] * 1e-8)) * g[i])
	}
}

// Adam runs the adam gradient descent algorithm.
func (to CPUOperator) Adam(
	params, gradients Tensor,
	vHistory, sHistory Tensor,
	smoothFactor1, smoothFactor2, learningRate float64,
) {
	p := params.(*SimpleTensor).data
	g := gradients.(*SimpleTensor).data
	s := sHistory.(*SimpleTensor).data
	v := vHistory.(*SimpleTensor).data
	for i := 0; i < len(p); i++ {
		v[i] = smoothFactor1*v[i] + (1-smoothFactor1)*g[i]
		s[i] = smoothFactor2*s[i] + (1-smoothFactor2)*g[i]*g[i]
		p[i] -= learningRate * (1.0 / (math.Sqrt(s[i]) + 1e-8)) * v[i]
	}
}

// ReluForward runs the ReLU forward propagation algorithm.
func (to CPUOperator) ReluForward(in Tensor) Tensor {
	out := to.Clone(in).(*SimpleTensor)

	for i := range out.data {
		if out.data[i] < 0 {
			out.data[i] = 0
		}
	}

	return out
}

// ReluBackward runs the relu backward propagation algorithm.
func (to CPUOperator) ReluBackward(forwardIn, backwardIn Tensor) Tensor {
	fIn := forwardIn.(*SimpleTensor).data
	out := to.Clone(backwardIn).(*SimpleTensor)

	for i := range out.data {
		if fIn[i] < 0 {
			out.data[i] = 0
		}
	}

	return out
}

func numElement(size []int) int {
	numElem := 1
	for _, s := range size {
		numElem *= s
	}

	return numElem
}

func (to CPUOperator) numElementMustMatch(t1, t2 Tensor) {
	if numElement(t1.Size()) != numElement(t2.Size()) {
		panic("number of element mismatch")
	}
}
