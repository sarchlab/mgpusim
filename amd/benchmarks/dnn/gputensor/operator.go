package gputensor

import (
	"fmt"
	"math"
	"sync"
	"time"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/benchmarks/dnn/tensor"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

var sizeOfFloat32 = 4
var sizeOfInt32 = 4

// GPUOperator can perform operations on GPU tensors.
type GPUOperator struct {
	driver        *driver.Driver
	ctx           *driver.Context
	Arch          arch.Type
	kernelsLoaded bool
	loadedArch    arch.Type
	verification  bool
	timerMutex    sync.Mutex
	reportTime    bool
	vStart, vEnd  sim.VTimeInSec
	start, end    time.Time
	cpuOperator   *tensor.CPUOperator

	sumKernel                           *insts.KernelCodeObject
	transposeKernel                     *insts.KernelCodeObject
	repeatKernel                        *insts.KernelCodeObject
	rotateKernel                        *insts.KernelCodeObject
	dilateKernel                        *insts.KernelCodeObject
	im2ColKernel                        *insts.KernelCodeObject
	softmaxExpKernel                    *insts.KernelCodeObject
	softmaxDivKernel                    *insts.KernelCodeObject
	scaleAddKernel                      *insts.KernelCodeObject
	elemWiseMulKernel                   *insts.KernelCodeObject
	rmsPropKernel                       *insts.KernelCodeObject
	adamKernel                          *insts.KernelCodeObject
	reluForwardKernel                   *insts.KernelCodeObject
	reluBackwardKernel                  *insts.KernelCodeObject
	maxPoolingForwardKernel             *insts.KernelCodeObject
	maxPoolingBackwardKernel            *insts.KernelCodeObject
	avgPoolingForwardKernel             *insts.KernelCodeObject
	avgPoolingBackwardKernel            *insts.KernelCodeObject
	gemmKernel                          *insts.KernelCodeObject
	crossEntropyDerivativeKernel        *insts.KernelCodeObject
	softmaxCrossEntropyDerivativeKernel *insts.KernelCodeObject
}

// NewGPUOperator creates a new GPU Operator.
func NewGPUOperator(
	gpuDriver *driver.Driver,
	ctx *driver.Context,
) *GPUOperator {
	o := &GPUOperator{
		driver: gpuDriver,
		ctx:    ctx,
	}

	return o
}

// ensureKernelsLoaded loads or reloads kernels if the architecture changed.
func (o *GPUOperator) ensureKernelsLoaded() {
	if !o.kernelsLoaded || o.loadedArch != o.Arch {
		o.loadKernels()
		o.kernelsLoaded = true
		o.loadedArch = o.Arch
	}
}

// EnableVerification will run the same operations in a CPU operator and
// compare the results.
func (o *GPUOperator) EnableVerification() {
	o.verification = true
	o.cpuOperator = &tensor.CPUOperator{}
}

// ReportTime lets the operator to report the execution time of each kernel.
func (o *GPUOperator) ReportTime() {
	o.reportTime = true
}

func (o *GPUOperator) gpuTensorToCPUTensor(
	t tensor.Tensor,
) *tensor.SimpleTensor {
	out := o.cpuOperator.CreateWithData(t.Vector(), t.Size(), t.Descriptor())
	return out.(*tensor.SimpleTensor)
}

func (o *GPUOperator) tensorMustMatch(expected, actual tensor.Tensor) {
	o.descriptorMustMatch(expected, actual)
	o.sizeMustMatch(expected, actual)
	o.valueMustMatch(expected, actual)
}

func (o *GPUOperator) descriptorMustMatch(expected, actual tensor.Tensor) {
	descriptorA := expected.Descriptor()
	descriptorB := actual.Descriptor()
	if descriptorA != descriptorB {
		fmt.Printf("Expected %s, but get %s\n", expected, actual)
		panic("discriptor not match")
	}
}

func (o *GPUOperator) sizeMustMatch(expected, actual tensor.Tensor) {
	sizeA := expected.Size()
	sizeB := actual.Size()
	if len(sizeA) != len(sizeB) {
		panic("dimension mismatch")
	}

	for i := range sizeA {
		if sizeA[i] != sizeB[i] {
			fmt.Printf("Expected %v, but get %v\n", sizeA, sizeB)
			panic("size mismatch")
		}
	}
}

func (o *GPUOperator) valueMustMatch(expected, actual tensor.Tensor) {
	expectedV := expected.Vector()
	actualV := actual.Vector()
	for i := range expectedV {
		if math.Abs(expectedV[i]) < 1e-5 && math.Abs(actualV[i]) < 1e-5 {
			//value too small
			continue
		}

		if math.Abs(expectedV[i]-actualV[i]) > math.Abs(1e-2*expectedV[i]) {
			fmt.Printf("At index %d, expected %.15f but get %.15f\n",
				i, expectedV[i], actualV[i])
			panic("value mismatch")
		}
	}
}

// GCN3 HSACO embeds
//
//go:embed operator.hsaco
var gcn3OperatorKernelBytes []byte

//go:embed repeat.hsaco
var gcn3RepeatKernelBytes []byte

//go:embed im2col.hsaco
var gcn3Im2ColKernelBytes []byte

//go:embed maxpooling.hsaco
var gcn3MaxPoolingKernelBytes []byte

//go:embed avgpooling.hsaco
var gcn3AvgPoolingKernelBytes []byte

//go:embed gemm.hsaco
var gcn3GemmKernelBytes []byte

//go:embed cross_entropy.hsaco
var gcn3CrossEntropyKernelBytes []byte

// CDNA3 (gfx942) HSACO embeds
//
//go:embed operator_gfx942.hsaco
var cdna3OperatorKernelBytes []byte

//go:embed repeat_gfx942.hsaco
var cdna3RepeatKernelBytes []byte

//go:embed im2col_gfx942.hsaco
var cdna3Im2ColKernelBytes []byte

//go:embed maxpooling_gfx942.hsaco
var cdna3MaxPoolingKernelBytes []byte

//go:embed avgpooling_gfx942.hsaco
var cdna3AvgPoolingKernelBytes []byte

//go:embed gemm_gfx942.hsaco
var cdna3GemmKernelBytes []byte

//go:embed cross_entropy_gfx942.hsaco
var cdna3CrossEntropyKernelBytes []byte

func (o *GPUOperator) loadKernels() {
	var opBytes, repeatBytes, im2colBytes []byte
	var maxpoolBytes, avgpoolBytes, gemmBytes, ceBytes []byte

	if o.Arch == arch.CDNA3 {
		opBytes = cdna3OperatorKernelBytes
		repeatBytes = cdna3RepeatKernelBytes
		im2colBytes = cdna3Im2ColKernelBytes
		maxpoolBytes = cdna3MaxPoolingKernelBytes
		avgpoolBytes = cdna3AvgPoolingKernelBytes
		gemmBytes = cdna3GemmKernelBytes
		ceBytes = cdna3CrossEntropyKernelBytes
	} else {
		opBytes = gcn3OperatorKernelBytes
		repeatBytes = gcn3RepeatKernelBytes
		im2colBytes = gcn3Im2ColKernelBytes
		maxpoolBytes = gcn3MaxPoolingKernelBytes
		avgpoolBytes = gcn3AvgPoolingKernelBytes
		gemmBytes = gcn3GemmKernelBytes
		ceBytes = gcn3CrossEntropyKernelBytes
	}

	loadKernel(&o.sumKernel, opBytes, "sum_one_axis")
	loadKernel(&o.transposeKernel, opBytes, "transpose_tensor")
	loadKernel(&o.rotateKernel, opBytes, "rotate_tensor")
	loadKernel(&o.dilateKernel, opBytes, "dilate_tensor")
	loadKernel(&o.softmaxExpKernel, opBytes, "softmax_exp")
	loadKernel(&o.softmaxDivKernel, opBytes, "softmax_div")
	loadKernel(&o.scaleAddKernel, opBytes, "scaleAdd")
	loadKernel(&o.elemWiseMulKernel, opBytes, "mul")
	loadKernel(&o.rmsPropKernel, opBytes, "rmsProp")
	loadKernel(&o.adamKernel, opBytes, "adam")
	loadKernel(&o.reluForwardKernel, opBytes, "reluForward")
	loadKernel(&o.reluBackwardKernel, opBytes, "reluBackward")
	loadKernel(&o.repeatKernel, repeatBytes, "repeat")
	loadKernel(&o.im2ColKernel, im2colBytes, "im2col_2d")
	loadKernel(&o.maxPoolingForwardKernel, maxpoolBytes, "MaxPoolForward")
	loadKernel(&o.maxPoolingBackwardKernel, maxpoolBytes, "MaxPoolBackward")
	loadKernel(&o.avgPoolingForwardKernel, avgpoolBytes, "AvgPoolForward")
	loadKernel(&o.avgPoolingBackwardKernel, avgpoolBytes, "AvgPoolBackward")
	loadKernel(&o.gemmKernel, gemmBytes, "gemm_old")
	loadKernel(&o.crossEntropyDerivativeKernel, ceBytes, "cross_entropy_derivative")
	loadKernel(&o.softmaxCrossEntropyDerivativeKernel, ceBytes, "softmax_cross_entropy_derivative")
}

func loadKernel(hsaco **insts.KernelCodeObject, kernelBytes []byte, name string) {
	*hsaco = insts.LoadKernelCodeObjectFromBytes(kernelBytes, name)
	if *hsaco == nil {
		panic("Failed to load " + name + "kernel")
	}
}

// Create creates a new GPU tensor
func (o *GPUOperator) Create(size []int) tensor.Tensor {
	t := &Tensor{
		driver: o.driver,
		ctx:    o.ctx,
		size:   size,
	}

	t.ptr = o.driver.AllocateMemory(o.ctx, uint64(t.NumElement()*sizeOfFloat32))

	return t
}

// CreateWithData creates the tensor and copies the given data to the GPU
// memory.
func (o *GPUOperator) CreateWithData(
	data []float64,
	size []int,
	descriptor string,
) tensor.Tensor {
	t := o.Create(size).(*Tensor)
	t.descriptor = descriptor

	f32Data := f64SliceToF32Slice(data)

	o.driver.MemCopyH2D(o.ctx, t.ptr, f32Data)

	return t
}

func f64SliceToF32Slice(in []float64) []float32 {
	f32Data := make([]float32, len(in))

	for i := 0; i < len(in); i++ {
		f32Data[i] = float32(in[i])
	}

	return f32Data
}

// Free releases the allocated GPU memory.
func (o *GPUOperator) Free(t tensor.Tensor) {
	o.driver.FreeMemory(o.ctx, t.(*Tensor).ptr)
	t.(*Tensor).ptr = 0
}

// Copy copies data from one tensor to another tensor. The src and dst tensor
// must have the same number of elements.
func (o *GPUOperator) Copy(dst tensor.Tensor, src tensor.Tensor) {
	d := dst.(*Tensor)
	s := src.(*Tensor)

	if d.NumElement() != s.NumElement() {
		panic(fmt.Sprintf("mismatch in size src size %v dst size %v",
			src.Size(), dst.Size()))
	}

	o.driver.MemCopyD2D(o.ctx, d.ptr, s.ptr, dst.NumElement()*sizeOfFloat32)
}

// Clone duplicates the input tensor.
func (o *GPUOperator) Clone(t tensor.Tensor) tensor.Tensor {
	inT := t.(*Tensor)
	outT := o.Create(t.Size()).(*Tensor)

	outT.size = make([]int, len(inT.size))
	copy(outT.size, inT.size)

	o.Copy(outT, inT)

	return outT
}

// Dump writes the content of the tensor to a string.
func (o *GPUOperator) Dump(t tensor.Tensor) string {
	v := make([]float32, t.NumElement())
	o.driver.MemCopyD2H(o.ctx, v, t.(*Tensor).ptr)

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

// Init sets the data of the tensor
func (o *GPUOperator) Init(t tensor.Tensor, data []float64) {
	if t.NumElement() != len(data) {
		panic("mismatch in buffer shape")
	}

	f32Data := f64SliceToF32Slice(data)

	o.driver.MemCopyH2D(o.ctx, t.(*Tensor).ptr, f32Data)
}

// Slice will create another tensor that shares part of the buffer with the
// input tensor.
func (o *GPUOperator) Slice(t tensor.Tensor, start int, end int) tensor.Tensor {
	out := &Tensor{
		driver: o.driver,
		ctx:    o.ctx,

		size: []int{end - start},
		ptr: driver.Ptr(uint64(t.(*Tensor).ptr) +
			uint64(start*sizeOfFloat32)),
	}

	return out
}

type repeatArgs struct {
	Output, Input             driver.Ptr
	InputLength, OutputLength uint32
	OffsetX, OffsetY, OffsetZ int64
}

// Repeat will create another tensor that duplicates the input tensor by n
// times.
func (o *GPUOperator) Repeat(t tensor.Tensor, times int) tensor.Tensor {
	o.ensureKernelsLoaded()
	numElem := t.NumElement()
	out := o.Create([]int{numElem * times}).(*Tensor)

	outLength := times * t.NumElement()
	args := repeatArgs{
		Output:       out.ptr,
		Input:        t.(*Tensor).ptr,
		InputLength:  uint32(t.NumElement()),
		OutputLength: uint32(outLength),
	}

	globalSize := [3]uint32{uint32(outLength), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	if o.Arch == arch.CDNA3 {
		o.launchRepeatCDNA3(globalSize, localSize,
			out.ptr, t.(*Tensor).ptr,
			uint32(t.NumElement()), uint32(outLength))
	} else {
		o.driver.LaunchKernel(o.ctx, o.repeatKernel,
			globalSize, localSize, &args)
	}

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.Repeat(cpuIn, times)
		o.tensorMustMatch(cpuOut, out)
		fmt.Println("Repeat verified.")
	}

	return out
}

// Clear sets the content of the tensor to 0.
func (o *GPUOperator) Clear(t tensor.Tensor) {
	data := make([]float64, t.NumElement())

	o.Init(t, data)
}

// Zeros creates a tensor prefilled with zeros.
func (o *GPUOperator) Zeros(size []int) tensor.Tensor {
	t := o.Create(size)

	o.Clear(t)

	return t
}

// Reshape creates another tensor with the same elements but a different size.
func (o *GPUOperator) Reshape(t tensor.Tensor, newSize []int) tensor.Tensor {
	out := o.Clone(t)

	out.SetSize(newSize)

	return out
}

type transposeKernelArgs struct {
	In, Out, InSize, OutSize, Order driver.Ptr
	InIndexBuf, OutIndexBuf         driver.Ptr
	Dim, Padding                    int32
	OffsetX, OffsetY, OffsetZ       int64
}

// Transpose reorders the axises of the tensor.
func (o *GPUOperator) Transpose(t tensor.Tensor, order []int) tensor.Tensor {
	o.ensureKernelsLoaded()
	input := t.(*Tensor)
	if len(order) != len(input.Size()) {
		panic("order should include all axes")
	}

	output, args, cleanup := o.prepareTranspose(t, order)
	defer cleanup()

	globalSize := [3]uint32{uint32(t.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchTransposeCDNA3(globalSize, localSize, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.transposeKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("Transpose")

	o.setTransposeOutputDescriptor(output, input, order)
	o.verifyTranspose(output, input, order)

	return output
}

func (o *GPUOperator) prepareTranspose(
	t tensor.Tensor, order []int,
) (*Tensor, transposeKernelArgs, func()) {
	dim := len(order)
	hOrder := make([]int32, dim)
	hInSize := make([]int32, dim)
	hOutSize := make([]int32, dim)
	outSize := make([]int, dim)
	for i := 0; i < dim; i++ {
		hOrder[i] = int32(order[i])
		hInSize[i] = int32(t.Size()[i])
		hOutSize[i] = int32(t.Size()[order[i]])
		outSize[i] = t.Size()[order[i]]
	}

	output := o.Create(outSize).(*Tensor)

	dOrder := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dOrder, hOrder)
	dInSize := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dInSize, hInSize)
	dOutSize := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dOutSize, hOutSize)
	dInIndexBuf := o.driver.AllocateMemory(o.ctx,
		uint64(t.NumElement()*dim*sizeOfInt32))
	dOutIndexBuf := o.driver.AllocateMemory(o.ctx,
		uint64(t.NumElement()*dim*sizeOfInt32))

	cleanup := func() {
		o.driver.FreeMemory(o.ctx, dOrder)
		o.driver.FreeMemory(o.ctx, dInSize)
		o.driver.FreeMemory(o.ctx, dOutSize)
		o.driver.FreeMemory(o.ctx, dInIndexBuf)
		o.driver.FreeMemory(o.ctx, dOutIndexBuf)
	}

	args := transposeKernelArgs{
		In: t.(*Tensor).ptr, Out: output.ptr,
		InSize: dInSize, OutSize: dOutSize, Order: dOrder,
		InIndexBuf: dInIndexBuf, OutIndexBuf: dOutIndexBuf,
		Dim: int32(len(order)),
	}

	return output, args, cleanup
}

func (o *GPUOperator) setTransposeOutputDescriptor(
	output, input *Tensor,
	order []int,
) {
	output.descriptor = ""
	for i := 0; i < len(input.Descriptor()); i++ {
		output.descriptor += string(input.Descriptor()[order[i]])
	}
}

func (o *GPUOperator) verifyTranspose(
	output, input *Tensor,
	order []int,
) {
	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(input)
		cpuOut := o.cpuOperator.Transpose(cpuIn, order)
		o.tensorMustMatch(cpuOut, output)
		fmt.Println("Transpose verified.")
	}
}

type rotateKernelArgs struct {
	In, Out, InSize, OutSize  driver.Ptr
	InIndexBuf, OutIndexBuf   driver.Ptr
	Dim, Padding              int32
	OffsetX, OffsetY, OffsetZ int64
}

// Rotate180 rotates the lowest two dimensions of the tensor by 180 degree.
func (o *GPUOperator) Rotate180(t tensor.Tensor) tensor.Tensor {
	o.ensureKernelsLoaded()
	dim := len(t.Size())
	hInSize := make([]int32, dim)
	hOutSize := make([]int32, dim)
	outSize := make([]int, dim)
	for i := 0; i < dim; i++ {
		hInSize[i] = int32(t.Size()[i])
		hOutSize[i] = int32(t.Size()[i])
		outSize[i] = t.Size()[i]
	}

	output := o.Create(outSize).(*Tensor)

	dInSize := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dInSize, hInSize)
	defer o.driver.FreeMemory(o.ctx, dInSize)
	dOutSize := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dOutSize, hOutSize)
	defer o.driver.FreeMemory(o.ctx, dOutSize)
	dInIndexBuf := o.driver.AllocateMemory(o.ctx,
		uint64(t.NumElement()*dim*sizeOfInt32))
	defer o.driver.FreeMemory(o.ctx, dInIndexBuf)
	dOutIndexBuf := o.driver.AllocateMemory(o.ctx,
		uint64(t.NumElement()*dim*sizeOfInt32))
	defer o.driver.FreeMemory(o.ctx, dOutIndexBuf)

	args := rotateKernelArgs{
		In:          t.(*Tensor).ptr,
		Out:         output.ptr,
		InSize:      dInSize,
		OutSize:     dOutSize,
		InIndexBuf:  dInIndexBuf,
		OutIndexBuf: dOutIndexBuf,
		Dim:         int32(len(t.Size())),
	}

	globalSize := [3]uint32{uint32(t.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchRotateCDNA3(globalSize, localSize, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.rotateKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("Rotate180")

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.Rotate180(cpuIn)
		o.tensorMustMatch(cpuOut, output)
		fmt.Println("Rotate180 verified.")
	}

	return output
}

type dilateKernelArgs struct {
	In, Out, InSize, OutSize  driver.Ptr
	Dilate                    driver.Ptr
	InIndexBuf, OutIndexBuf   driver.Ptr
	Dim, Padding              int32
	OffsetX, OffsetY, OffsetZ int64
}

// Dilate adds 0s between rows and columns.
func (o *GPUOperator) Dilate(t tensor.Tensor, dilate []int) tensor.Tensor {
	o.ensureKernelsLoaded()
	dim := len(t.Size())
	hDilate := []int32{int32(dilate[0]), int32(dilate[1])}

	outSize := make([]int, len(t.Size()))
	copy(outSize, t.Size())

	outSize[len(outSize)-1] = (outSize[len(outSize)-1]-1)*dilate[1] + 1
	outSize[len(outSize)-2] = (outSize[len(outSize)-2]-1)*dilate[0] + 1
	output := o.Create(outSize).(*Tensor)

	hInSize := make([]int32, dim)
	hOutSize := make([]int32, dim)
	for i := 0; i < dim; i++ {
		hInSize[i] = int32(t.Size()[i])
		hOutSize[i] = int32(outSize[i])
	}

	dInSize := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dInSize, hInSize)
	defer o.driver.FreeMemory(o.ctx, dInSize)
	dOutSize := o.driver.AllocateMemory(o.ctx, uint64(dim*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dOutSize, hOutSize)
	defer o.driver.FreeMemory(o.ctx, dOutSize)
	dDilate := o.driver.AllocateMemory(o.ctx, uint64(2*sizeOfInt32))
	o.driver.MemCopyH2D(o.ctx, dDilate, hDilate)
	defer o.driver.FreeMemory(o.ctx, dDilate)
	dInIndexBuf := o.driver.AllocateMemory(o.ctx,
		uint64(output.NumElement()*dim*sizeOfInt32))
	defer o.driver.FreeMemory(o.ctx, dInIndexBuf)
	dOutIndexBuf := o.driver.AllocateMemory(o.ctx,
		uint64(output.NumElement()*dim*sizeOfInt32))
	defer o.driver.FreeMemory(o.ctx, dOutIndexBuf)

	args := dilateKernelArgs{
		In:          t.(*Tensor).ptr,
		Out:         output.ptr,
		InSize:      dInSize,
		OutSize:     dOutSize,
		Dilate:      dDilate,
		InIndexBuf:  dInIndexBuf,
		OutIndexBuf: dOutIndexBuf,
		Dim:         int32(len(t.Size())),
	}

	globalSize := [3]uint32{uint32(output.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchDilateCDNA3(globalSize, localSize, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.dilateKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("Dilate")

	o.verifyDilate(t, dilate, output)

	return output
}

func (o *GPUOperator) verifyDilate(t tensor.Tensor, dilate []int, output *Tensor) {
	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.Dilate(cpuIn, dilate)
		o.tensorMustMatch(cpuOut, output)
		fmt.Println("Dilate verified.")
	}
}

// Sum reduces the number of axes by summing the numbers on given axes.
func (o *GPUOperator) Sum(t tensor.Tensor, axis []int) tensor.Tensor {
	o.ensureKernelsLoaded()
	var in, out tensor.Tensor

	o.axisMustBeIncreasing(axis)

	in = t
	for i, a := range axis {
		out = o.sumOneAxis(in, a-i)

		if i > 0 {
			o.Free(in)
		}

		in = out
	}

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.Sum(cpuIn, axis)
		o.tensorMustMatch(cpuOut, out)
		fmt.Println("Sum verified.")
	}

	return out
}

func (o *GPUOperator) axisMustBeIncreasing(axis []int) {
	for i := 1; i < len(axis); i++ {
		if axis[i] < axis[i-1] {
			panic("axis not increasing")
		}
	}
}

type sumOneAxisKernelArgs struct {
	In, Out, InSize, OutSize  driver.Ptr
	InDim, Axis               int32
	InIndexBuf, OutIndexBuf   driver.Ptr
	OffsetX, OffsetY, OffsetZ int64
}

func (o *GPUOperator) sumOneAxis(t tensor.Tensor, axis int) tensor.Tensor {
	outSize := make([]int, 0)
	for i := range t.Size() {
		if i != axis {
			outSize = append(outSize, t.Size()[i])
		}
	}

	out := o.Create(outSize)
	args, cleanup := o.prepareSumOneAxis(t, out, outSize, axis)
	defer cleanup()

	gs := [3]uint32{uint32(out.NumElement()), 1, 1}
	ls := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchSumOneAxisCDNA3(gs, ls, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.sumKernel, gs, ls, &args)
	}
	o.timerEnd("Sum")

	return out
}

func (o *GPUOperator) prepareSumOneAxis(
	t, out tensor.Tensor, outSize []int, axis int,
) (sumOneAxisKernelArgs, func()) {
	hOutSize := make([]int32, len(outSize))
	for i := range outSize {
		hOutSize[i] = int32(outSize[i])
	}
	hInSize := make([]int32, len(t.Size()))
	for i := range t.Size() {
		hInSize[i] = int32(t.Size()[i])
	}

	globalSize := out.NumElement()
	dInSize := o.driver.AllocateMemory(o.ctx, uint64(t.Dim()*4))
	o.driver.MemCopyH2D(o.ctx, dInSize, hInSize)
	dOutSize := o.driver.AllocateMemory(o.ctx, uint64(len(outSize)*4))
	o.driver.MemCopyH2D(o.ctx, dOutSize, hOutSize)
	dInIndexBuf := o.driver.AllocateMemory(o.ctx, uint64(globalSize*t.Dim()*4))
	dOutIndexBuf := o.driver.AllocateMemory(o.ctx, uint64(globalSize*out.Dim()*4))

	cleanup := func() {
		o.driver.FreeMemory(o.ctx, dInSize)
		o.driver.FreeMemory(o.ctx, dOutSize)
		o.driver.FreeMemory(o.ctx, dInIndexBuf)
		o.driver.FreeMemory(o.ctx, dOutIndexBuf)
	}

	args := sumOneAxisKernelArgs{
		In: t.(*Tensor).ptr, Out: out.(*Tensor).ptr,
		InSize: dInSize, OutSize: dOutSize,
		InDim: int32(t.Dim()), Axis: int32(axis),
		InIndexBuf: dInIndexBuf, OutIndexBuf: dOutIndexBuf,
	}

	return args, cleanup
}

type gemmKernArgs struct {
	M, N, K                   int32
	Alpha, Beta               float32
	Padding                   int32
	A, B, C, D                driver.Ptr
	OffsetX, OffsetY, OffsetZ int32
}

// Gemm performs alpha * A * B + beta * C operation.
func (o *GPUOperator) Gemm(
	transA, transB bool,
	alpha, beta float64,
	a, b, c tensor.Tensor,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	tempA := a
	if transA {
		tempA = o.Transpose(a, []int{1, 0})
	}

	tempB := b
	if transB {
		tempB = o.Transpose(b, []int{1, 0})
	}

	d := o.matrixMultiplication(alpha, beta, tempA, tempB, c)

	if transA {
		o.Free(tempA)
	}

	if transB {
		o.Free(tempB)
	}

	if o.verification {
		cpuA := o.gpuTensorToCPUTensor(a)
		cpuB := o.gpuTensorToCPUTensor(b)
		cpuC := o.gpuTensorToCPUTensor(c)
		cpuOut := o.cpuOperator.Gemm(
			transA, transB, alpha, beta, cpuA, cpuB, cpuC)
		o.tensorMustMatch(cpuOut, d)
		fmt.Println("Gemm verified.")
	}

	return d
}

func (o *GPUOperator) matrixMultiplication(
	alpha, beta float64,
	a, b, c tensor.Tensor,
) tensor.Tensor {
	o.gemmDimMustBeValid(a, b, c)

	m := a.Size()[0]
	n := b.Size()[1]
	k := b.Size()[0]

	blockSize := 16
	wiWidth := ((n-1)/blockSize + 1) * blockSize
	wiHeight := ((m-1)/blockSize + 1) * blockSize

	d := o.Create([]int{m, n})

	kernArg := gemmKernArgs{
		M:     int32(m),
		N:     int32(n),
		K:     int32(k),
		Alpha: float32(alpha),
		Beta:  float32(beta),
		A:     a.(*Tensor).ptr,
		B:     b.(*Tensor).ptr,
		C:     c.(*Tensor).ptr,
		D:     d.(*Tensor).ptr,
	}

	globalSize := [3]uint32{uint32(wiWidth), uint32(wiHeight), 1}
	localSize := [3]uint16{uint16(blockSize), uint16(blockSize), 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchGemmCDNA3(globalSize, localSize, &kernArg)
	} else {
		o.driver.LaunchKernel(o.ctx, o.gemmKernel,
			globalSize, localSize, &kernArg)
	}
	o.timerEnd("Gemm")

	return d
}

func (o *GPUOperator) gemmDimMustBeValid(a, b, c tensor.Tensor) {
	if a.Dim() != 2 {
		panic("not a matrix")
	}

	if b.Dim() != 2 {
		panic("not a matrix")
	}

	if c.Dim() != 2 {
		panic("not a matrix")
	}

	if a.Size()[1] != b.Size()[0] {
		panic("width of matrix A does not match height of matrix B")
	}

	if a.Size()[0] != c.Size()[0] || b.Size()[1] != c.Size()[1] {
		panic("matrix C size mismatch")
	}
}

type im2ColKernelArg struct {
	Input, Output             driver.Ptr
	InputDim, MaskDim         [2]uint32
	Stride, Pad, Dilation     [2]uint32
	Channel, Batch            uint32
	OffsetX, OffsetY, OffsetZ int64
}

// Im2Col converts images to columns so that convolutional operations can be
// completed with GEMM.
func (o *GPUOperator) Im2Col(
	t tensor.Tensor,
	kernelSize, padding, stride, dilation []int,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	inputSize := t.Size()

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

	output := o.Create([]int{outHeight, outWidth})

	kernArg := im2ColKernelArg{
		Input:    t.(*Tensor).ptr,
		Output:   output.(*Tensor).ptr,
		InputDim: [2]uint32{uint32(inputSize[3]), uint32(inputSize[2])},
		MaskDim:  [2]uint32{uint32(kernelSize[1]), uint32(kernelSize[0])},
		Stride:   [2]uint32{uint32(stride[1]), uint32(stride[0])},
		Pad:      [2]uint32{uint32(padding[1]), uint32(padding[0])},
		Dilation: [2]uint32{uint32(dilation[1]), uint32(dilation[0])},
		Channel:  uint32(inputSize[1]),
		Batch:    uint32(inputSize[0]),
	}

	fmt.Printf("Im2Col input %v, kernel %v, stride %v, padding %v, "+
		"dilation %v, output %v\n",
		inputSize, kernelSize, stride, padding, dilation, output.Size())

	globalSize := [3]uint32{uint32(outWidth), uint32(outHeight), 1}
	localSize := [3]uint16{uint16(8), uint16(8), 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchIm2ColCDNA3(globalSize, localSize, &kernArg)
	} else {
		o.driver.LaunchKernel(o.ctx, o.im2ColKernel,
			globalSize, localSize, &kernArg)
	}
	o.timerEnd("Im2Col")

	o.verifyIm2Col(t, output, kernelSize, padding, stride, dilation)

	return output
}

func (o *GPUOperator) verifyIm2Col(
	t, output tensor.Tensor,
	kernelSize, padding, stride, dilation []int,
) {
	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.Im2Col(cpuIn,
			kernelSize, padding, stride, dilation)
		o.tensorMustMatch(cpuOut, output)
		fmt.Println("Im2Col verified.")
	}
}

func (o *GPUOperator) timerStart() {
	if o.reportTime {
		o.timerMutex.Lock()
		o.vStart = o.driver.Engine.CurrentTime()
		o.start = time.Now()
	}
}

func (o *GPUOperator) timerEnd(
	kernelName string,
) {
	if o.reportTime {
		o.vEnd = o.driver.Engine.CurrentTime()
		o.end = time.Now()

		fmt.Printf("Kernel %s, Start %.10f, Virtual Time: %v, Real Time: %v\n",
			kernelName, o.vStart, o.vEnd-o.vStart, o.end.Sub(o.start))
		o.timerMutex.Unlock()
	}
}

type maxPoolingForwardKernelArgs struct {
	NThreads, Padding            int32
	BottomData                   driver.Ptr
	Num, Channels, Height, Width int32
	PooledH, PooledW             int32
	KernelH, KernelW             int32
	StrideH, StrideW             int32
	PadH, PadW                   int32
	TopData, MaskData            driver.Ptr
	OffsetX, OffsetY, OffsetZ    int64
}

// MaxPoolingForward calculates the forward propagation of the max pooling
// layer.
func (o *GPUOperator) MaxPoolingForward(
	t tensor.Tensor,
	kernelSize, padding, stride []int,
) (out tensor.Tensor, mask tensor.Tensor) {
	o.ensureKernelsLoaded()
	input := t.(*Tensor)
	n := input.size[0]
	c := input.size[1]
	hIn := input.size[2]
	wIn := input.size[3]

	hOut := (hIn+2*padding[0]-kernelSize[0])/stride[0] + 1
	wOut := (wIn+2*padding[1]-kernelSize[1])/stride[1] + 1

	outT := o.Create([]int{n, c, hOut, wOut}).(*Tensor)
	maskT := o.Create([]int{n, c, hOut, wOut}).(*Tensor)

	kernArg := maxPoolingForwardKernelArgs{
		NThreads:   int32(n * c * hOut * wOut),
		BottomData: input.ptr,
		Num:        int32(n),
		Channels:   int32(c),
		Height:     int32(hIn),
		Width:      int32(wIn),
		PooledH:    int32(hOut),
		PooledW:    int32(wOut),
		KernelH:    int32(kernelSize[0]),
		KernelW:    int32(kernelSize[1]),
		StrideH:    int32(stride[0]),
		StrideW:    int32(stride[1]),
		PadH:       int32(padding[0]),
		PadW:       int32(padding[1]),
		TopData:    outT.ptr,
		MaskData:   maskT.ptr,
	}

	globalSize := [3]uint32{uint32(n * c * hOut * wOut), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchMaxPoolFwdCDNA3(globalSize, localSize, &kernArg)
	} else {
		o.driver.LaunchKernel(o.ctx, o.maxPoolingForwardKernel,
			globalSize, localSize, &kernArg)
	}
	o.timerEnd("MaxPoolingForward")

	return outT, maskT
}

type maxPoolingBackwardKernelArgs struct {
	NThreads, Padding            int32
	TopDiff, TopMask             driver.Ptr
	Num, Channels, Height, Width int32
	PooledHeight, PooledWidth    int32
	KernelH, KernelW             int32
	StrideH, StrideW             int32
	PadH, PadW                   int32
	BottomDiff                   driver.Ptr
	OffsetX, OffsetY, OffsetZ    int64
}

// MaxPoolingBackward calculates the backward propagation of the max pooling
// layer.
func (o *GPUOperator) MaxPoolingBackward(
	forwardIn, backwardIn tensor.Tensor,
	mask tensor.Tensor,
	kernelSize, padding, stride []int,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	n := forwardIn.Size()[0]
	c := forwardIn.Size()[1]
	hIn := forwardIn.Size()[2]
	wIn := forwardIn.Size()[3]
	hOut := backwardIn.Size()[2]
	wOut := backwardIn.Size()[3]

	out := o.Create([]int{n, c, hIn, wIn})

	kernArg := maxPoolingBackwardKernelArgs{
		NThreads:     int32(n * c * hIn * hOut),
		TopDiff:      backwardIn.(*Tensor).ptr,
		TopMask:      mask.(*Tensor).ptr,
		Num:          int32(n),
		Channels:     int32(c),
		Height:       int32(hIn),
		Width:        int32(wIn),
		PooledHeight: int32(hOut),
		PooledWidth:  int32(wOut),
		KernelH:      int32(kernelSize[0]),
		KernelW:      int32(kernelSize[1]),
		StrideH:      int32(stride[0]),
		StrideW:      int32(stride[1]),
		PadH:         int32(padding[0]),
		PadW:         int32(padding[1]),
		BottomDiff:   out.(*Tensor).ptr,
	}

	globalSize := [3]uint32{uint32(n * c * hIn * wIn), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchMaxPoolBwdCDNA3(globalSize, localSize, &kernArg)
	} else {
		o.driver.LaunchKernel(o.ctx, o.maxPoolingBackwardKernel,
			globalSize, localSize, &kernArg)
	}
	o.timerEnd("MaxPoolingBackward")

	return out
}

// AvgPoolingKernelArgsForward defines forward kernel arguments
type AvgPoolingKernelArgsForward struct {
	NumThreads uint64
	Bottom     driver.Ptr
	N          int32
	C          int32
	H          int32
	W          int32
	PooledH    int32
	PooledW    int32
	KernelH    int32
	KernelW    int32
	StrideH    int32
	StrideW    int32
	PadH       int32
	PadW       int32
	Top        driver.Ptr

	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// AvgPoolingForward calculates the forward propagation of the average pooling
// layer.
func (o *GPUOperator) AvgPoolingForward(
	t tensor.Tensor,
	kernelSize, padding, stride []int,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	input := t.(*Tensor)
	B := input.size[0]
	C := input.size[1]
	Hin := input.size[2]
	Win := input.size[3]
	ks := kernelSize
	Hout := (Hin+2*padding[0]-ks[0])/stride[0] + 1
	Wout := (Win+2*padding[1]-ks[1])/stride[1] + 1
	output := o.Create([]int{B, C, Hout, Wout}).(*Tensor)

	kernArg := AvgPoolingKernelArgsForward{
		uint64(B * C * Hout * Wout), input.ptr,
		int32(B), int32(C), int32(Hin), int32(Win),
		int32(Hout), int32(Wout),
		int32(ks[0]), int32(ks[1]),
		int32(stride[0]), int32(stride[1]),
		int32(padding[0]), int32(padding[1]),
		output.ptr,
		0, 0, 0,
	}

	globalSize := [3]uint32{uint32(B * C * Hout * Wout), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchAvgPoolFwdCDNA3(globalSize, localSize, &kernArg)
	} else {
		o.driver.LaunchKernel(o.ctx, o.avgPoolingForwardKernel,
			globalSize, localSize, &kernArg)
	}
	o.timerEnd("AvgPoolingForward")

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.AvgPoolingForward(cpuIn,
			kernelSize, padding, stride)

		o.tensorMustMatch(cpuOut, output)
		fmt.Println("AvgPoolingForward verified.")
	}

	return output
}

// AvgPoolingKernelArgsBackward defines forward kernel arguments
type AvgPoolingKernelArgsBackward struct {
	NumThreads uint64
	Top        driver.Ptr
	N          int32
	C          int32
	H          int32
	W          int32
	PooledH    int32
	PooledW    int32
	KernelH    int32
	KernelW    int32
	StrideH    int32
	StrideW    int32
	PadH       int32
	PadW       int32
	Bottom     driver.Ptr

	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// AvgPoolingBackward claculates the backward propagation of the average pooling
// layer.
func (o *GPUOperator) AvgPoolingBackward(
	forwardIn, backwardIn tensor.Tensor,
	kernelSize, padding, stride []int,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	input := backwardIn
	ks := kernelSize
	B := forwardIn.Size()[0]
	C := forwardIn.Size()[1]
	Hin := forwardIn.Size()[2]
	Hout := backwardIn.Size()[2]
	Win := forwardIn.Size()[3]
	Wout := backwardIn.Size()[3]

	output := o.Create([]int{B, C, Hin, Win}).(*Tensor)

	kernArg := AvgPoolingKernelArgsBackward{
		uint64(B * C * Hin * Win), input.(*Tensor).ptr,
		int32(B), int32(C), int32(Hin), int32(Win),
		int32(Hout), int32(Wout),
		int32(ks[0]), int32(ks[1]),
		int32(stride[0]), int32(stride[1]),
		int32(padding[0]), int32(padding[1]),
		output.ptr,
		0, 0, 0,
	}

	globalSize := [3]uint32{uint32(B * C * Hin * Win), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchAvgPoolBwdCDNA3(globalSize, localSize, &kernArg)
	} else {
		o.driver.LaunchKernel(o.ctx, o.avgPoolingBackwardKernel,
			globalSize, localSize, &kernArg)
	}
	o.timerEnd("AvgPoolingBackward")

	if o.verification {
		cpuForwardIn := o.gpuTensorToCPUTensor(forwardIn)
		cpuBackwardIn := o.gpuTensorToCPUTensor(backwardIn)
		cpuOut := o.cpuOperator.AvgPoolingBackward(
			cpuForwardIn, cpuBackwardIn,
			kernelSize, padding, stride)

		o.tensorMustMatch(cpuOut, output)
		fmt.Println("AvgPoolingBackward verified.")
	}

	return output
}

type softmaxExpKernelArg struct {
	Input                     driver.Ptr
	Output                    driver.Ptr
	N, Padding                int32
	OffsetX, OffsetY, OffsetZ int64
}

type reductionSumKernelArg struct {
	Data                      driver.Ptr
	PartialSums               driver.LocalPtr
	Padding                   int32
	Output                    driver.Ptr
	InputN, Padding2          int32
	OffsetX, OffsetY, OffsetZ int64
}

type softmaxDivKernelArg struct {
	ExpInput                  driver.Ptr
	Output                    driver.Ptr
	Denominator               driver.Ptr
	NumElement, BatchSize     int32
	OffsetX, OffsetY, OffsetZ int64
}

// Softmax performs the softmax operation.
func (o *GPUOperator) Softmax(t tensor.Tensor) tensor.Tensor {
	o.ensureKernelsLoaded()
	o.mustBeTwoDimension(t)

	input := t.(*Tensor)
	output := o.Create(input.size).(*Tensor)
	expInput := o.Create(
		[]int{input.size[0], t.NumElement() / input.size[0]},
	).(*Tensor)
	defer o.Free(expInput)

	expGlobalSize := [3]uint32{uint32(input.NumElement()), 1, 1}
	expLocalSize := [3]uint16{64, 1, 1}

	if o.Arch == arch.CDNA3 {
		o.launchSoftmaxExpCDNA3(expGlobalSize, expLocalSize,
			input.ptr, expInput.ptr, int32(input.NumElement()))
	} else {
		expArgs := softmaxExpKernelArg{
			Input:  input.ptr,
			Output: expInput.ptr,
			N:      int32(input.NumElement()),
		}
		o.driver.LaunchKernel(o.ctx, o.softmaxExpKernel,
			expGlobalSize, expLocalSize, &expArgs)
	}

	denominator := o.Sum(expInput, []int{1})

	divGlobalSize := [3]uint32{uint32(expInput.NumElement()), 1, 1}
	divLocalSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchSoftmaxDivCDNA3(divGlobalSize, divLocalSize,
			expInput.ptr, output.ptr, denominator.(*Tensor).ptr,
			int32(expInput.NumElement()), int32(t.Size()[0]))
	} else {
		divArgs := softmaxDivKernelArg{
			ExpInput:    expInput.ptr,
			Output:      output.ptr,
			Denominator: denominator.(*Tensor).ptr,
			NumElement:  int32(expInput.NumElement()),
			BatchSize:   int32(t.Size()[0]),
		}
		o.driver.LaunchKernel(o.ctx, o.softmaxDivKernel,
			divGlobalSize, divLocalSize, &divArgs)
	}
	o.timerEnd("Softmax")

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.Softmax(cpuIn)

		o.tensorMustMatch(cpuOut, output)
		fmt.Println("Softmax verified.")
	}

	return output
}

func (o *GPUOperator) mustBeTwoDimension(t tensor.Tensor) {
	if t.Dim() != 2 {
		panic("Tensor is not two dimension")
	}
}

// CrossEntropy calculates the cross entropy of the output.
func (o *GPUOperator) CrossEntropy(t tensor.Tensor, label []int) float64 {
	o.ensureKernelsLoaded()
	o.mustBeTwoDimension(t)

	loss := 0.0
	inputV := t.Vector()
	for i := 0; i < t.Size()[0]; i++ {
		start := i * t.Size()[1]
		end := start + t.Size()[1]

		inputSlice := inputV[start:end]

		loss += -math.Log(inputSlice[label[i]])
	}

	loss /= float64(t.Size()[0])

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.CrossEntropy(cpuIn, label)

		if cpuOut != loss {
			panic("mismatch")
		}

		fmt.Println("CrossEntropy verified.")
	}

	return loss
}

type crossEntropyDerivativeArgs struct {
	Output, Input, Label      driver.Ptr
	BatchSize, NumPerImage    int32
	OffsetX, OffsetY, OffsetZ int64
}

// CrossEntropyDerivative calculates the derivative using cross entropies.
func (o *GPUOperator) CrossEntropyDerivative(
	t tensor.Tensor, label []int,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	hLabel := make([]int32, len(label))
	for i := 0; i < len(label); i++ {
		hLabel[i] = int32(label[i])
	}
	dLabel := o.driver.AllocateMemory(o.ctx, uint64(len(label)*4))
	defer o.driver.FreeMemory(o.ctx, dLabel)
	o.driver.MemCopyH2D(o.ctx, dLabel, hLabel)

	output := o.Create(t.Size()).(*Tensor)

	args := crossEntropyDerivativeArgs{
		Output:      output.ptr,
		Input:       t.(*Tensor).ptr,
		Label:       dLabel,
		BatchSize:   int32(t.Size()[0]),
		NumPerImage: int32(t.Size()[1]),
	}

	globalSize := [3]uint32{uint32(t.Size()[0] * t.Size()[1]), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchCrossEntropyDerivCDNA3(globalSize, localSize,
			o.crossEntropyDerivativeKernel, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.crossEntropyDerivativeKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("CrossEntropyDerivative")

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.CrossEntropyDerivative(cpuIn, label)

		o.tensorMustMatch(cpuOut, output)
		fmt.Println("CrossEntropyDerivative verified.")
	}

	return output
}

// SoftmaxCrossEntropyDerivative calculates the derivatives using both softmax /// and cross entropy algorithms.
func (o *GPUOperator) SoftmaxCrossEntropyDerivative(
	t tensor.Tensor,
	label []int,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	hLabel := make([]int32, len(label))
	for i := 0; i < len(label); i++ {
		hLabel[i] = int32(label[i])
	}
	dLabel := o.driver.AllocateMemory(o.ctx, uint64(len(label)*4))
	defer o.driver.FreeMemory(o.ctx, dLabel)
	o.driver.MemCopyH2D(o.ctx, dLabel, hLabel)

	output := o.Create(t.Size()).(*Tensor)

	args := crossEntropyDerivativeArgs{
		Output:      output.ptr,
		Input:       t.(*Tensor).ptr,
		Label:       dLabel,
		BatchSize:   int32(t.Size()[0]),
		NumPerImage: int32(t.Size()[1]),
	}

	globalSize := [3]uint32{uint32(t.Size()[0] * t.Size()[1]), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchCrossEntropyDerivCDNA3(globalSize, localSize,
			o.softmaxCrossEntropyDerivativeKernel, &args)
	} else {
		o.driver.LaunchKernel(o.ctx,
			o.softmaxCrossEntropyDerivativeKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("SoftmaxCrossEntropyDerivative")

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(t)
		cpuOut := o.cpuOperator.SoftmaxCrossEntropyDerivative(cpuIn, label)
		o.tensorMustMatch(cpuOut, output)
		fmt.Println("SoftmaxCrossEntropyDerivative verified.")
	}

	return output
}

type elemWiseMulKernArg struct {
	Out, In1, In2             driver.Ptr
	N, Padding                int32
	OffsetX, OffsetY, OffsetZ int64
}

// ElementWiseMul calculates the element multiplication of A and B.
func (o *GPUOperator) ElementWiseMul(
	a, b tensor.Tensor,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	if a.NumElement() != b.NumElement() {
		panic("size not match")
	}

	out := o.Create(a.Size()).(*Tensor)
	args := elemWiseMulKernArg{
		Out: out.ptr,
		In1: a.(*Tensor).ptr,
		In2: b.(*Tensor).ptr,
		N:   int32(a.NumElement()),
	}

	globalSize := [3]uint32{uint32(a.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchElemWiseMulCDNA3(globalSize, localSize,
			out.ptr, a.(*Tensor).ptr, b.(*Tensor).ptr,
			int32(a.NumElement()))
	} else {
		o.driver.LaunchKernel(o.ctx, o.elemWiseMulKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("ElementWiseMul")

	if o.verification {
		cpuA := o.gpuTensorToCPUTensor(a)
		cpuB := o.gpuTensorToCPUTensor(a)
		cpuOut := o.cpuOperator.ElementWiseMul(cpuA, cpuB)
		o.tensorMustMatch(cpuOut, out)
		fmt.Println("ElementWiseMul verified.")
	}

	return out
}

type scaleAddKernArg struct {
	Out, In1, In2             driver.Ptr
	Alpha, Beta               float32
	N, Padding                int32
	OffsetX, OffsetY, OffsetZ int64
}

// ScaleAdd performs element-wide alpha*A + beta*B operation.
func (o *GPUOperator) ScaleAdd(
	alpha, beta float64,
	a, b tensor.Tensor,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	if a.NumElement() != b.NumElement() {
		panic("size not match")
	}

	out := o.Create(a.Size()).(*Tensor)
	args := scaleAddKernArg{
		Out:   out.ptr,
		In1:   a.(*Tensor).ptr,
		In2:   b.(*Tensor).ptr,
		Alpha: float32(alpha),
		Beta:  float32(beta),
		N:     int32(a.NumElement()),
	}

	globalSize := [3]uint32{uint32(a.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchScaleAddCDNA3(globalSize, localSize, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.scaleAddKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("ScaleAdd")

	if o.verification {
		cpuA := o.gpuTensorToCPUTensor(a)
		cpuB := o.gpuTensorToCPUTensor(a)
		cpuOut := o.cpuOperator.ScaleAdd(alpha, beta, cpuA, cpuB)
		o.tensorMustMatch(cpuOut, out)
		fmt.Println("ScaleAdd verified.")
	}

	return out
}

type rmsPropKernArg struct {
	Params, Gradients, SHistory driver.Ptr
	SmoothFactor, LearningRate  float32
	N, Padding                  int32
	OffsetX, OffsetY, OffsetZ   int64
}

// RMSProp uses the RMSProp algorithm to update the parameters
func (o *GPUOperator) RMSProp(
	params, gradient, sHistory tensor.Tensor,
	smoothFactor, learningRate float64,
) {
	o.ensureKernelsLoaded()
	if params.NumElement() != gradient.NumElement() ||
		params.NumElement() != sHistory.NumElement() {
		panic("size mismatch")
	}

	args := rmsPropKernArg{
		Params:       params.(*Tensor).ptr,
		Gradients:    gradient.(*Tensor).ptr,
		SHistory:     sHistory.(*Tensor).ptr,
		SmoothFactor: float32(smoothFactor),
		LearningRate: float32(learningRate),
		N:            int32(params.NumElement()),
	}

	globalSize := [3]uint32{uint32(params.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchRmsPropCDNA3(globalSize, localSize, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.rmsPropKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("RMSProp")
}

type adamKernArg struct {
	Params, Gradients, SHistory, VHistory      driver.Ptr
	SmoothFactor1, SmoothFactor2, LearningRate float32
	N                                          int32
	OffsetX, OffsetY, OffsetZ                  int64
}

// Adam uses the Adam algorithm to update the parameters
func (o *GPUOperator) Adam(
	params, gradient, vHistory, sHistory tensor.Tensor,
	smoothFactor1, smoothFactor2, learningRate float64,
) {
	o.ensureKernelsLoaded()
	if params.NumElement() != gradient.NumElement() ||
		params.NumElement() != sHistory.NumElement() ||
		params.NumElement() != vHistory.NumElement() {
		panic("size mismatch")
	}

	var cpuParams, cpuGradient, cpuSHistory, cpuVHistory *tensor.SimpleTensor
	if o.verification {
		cpuParams = o.gpuTensorToCPUTensor(params)
		cpuGradient = o.gpuTensorToCPUTensor(gradient)
		cpuSHistory = o.gpuTensorToCPUTensor(sHistory)
		cpuVHistory = o.gpuTensorToCPUTensor(vHistory)
	}

	args := adamKernArg{
		Params:        params.(*Tensor).ptr,
		Gradients:     gradient.(*Tensor).ptr,
		SHistory:      sHistory.(*Tensor).ptr,
		VHistory:      vHistory.(*Tensor).ptr,
		SmoothFactor1: float32(smoothFactor1),
		SmoothFactor2: float32(smoothFactor2),
		LearningRate:  float32(learningRate),
		N:             int32(params.NumElement()),
	}

	globalSize := [3]uint32{uint32(params.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchAdamCDNA3(globalSize, localSize, &args)
	} else {
		o.driver.LaunchKernel(o.ctx, o.adamKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("Adam")

	if o.verification {
		o.cpuOperator.Adam(cpuParams, cpuGradient, cpuVHistory, cpuSHistory, smoothFactor1, smoothFactor2, learningRate)

		o.tensorMustMatch(cpuVHistory, vHistory)
		o.tensorMustMatch(cpuSHistory, sHistory)
		o.tensorMustMatch(cpuParams, params)
	}
}

type reluForwardKernelArgs struct {
	In, Out                   driver.Ptr
	Count, Padding            int32
	OffsetX, OffsetY, OffsetZ int64
}

// ReluForward Implementation
func (o *GPUOperator) ReluForward(
	in tensor.Tensor,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	out := o.Create(in.Size()).(*Tensor)
	out.descriptor = in.Descriptor()

	args := reluForwardKernelArgs{
		In:    in.(*Tensor).ptr,
		Out:   out.ptr,
		Count: int32(in.NumElement()),
	}

	globalSize := [3]uint32{uint32(in.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchReluForwardCDNA3(globalSize, localSize,
			in.(*Tensor).ptr, out.ptr, int32(in.NumElement()))
	} else {
		o.driver.LaunchKernel(o.ctx, o.reluForwardKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("ReluForward")

	if o.verification {
		cpuIn := o.gpuTensorToCPUTensor(in)
		cpuOut := o.cpuOperator.ReluForward(cpuIn)
		o.tensorMustMatch(cpuOut, out)
		fmt.Println("ReluForward verified.")
	}

	return out
}

type reluBackwardKernelArgs struct {
	In, Backin, Out           driver.Ptr
	Count, Padding            int32
	OffsetX, OffsetY, OffsetZ int64
}

// ReluBackward Implementation
func (o *GPUOperator) ReluBackward(
	forwardIn, backIn tensor.Tensor,
) tensor.Tensor {
	o.ensureKernelsLoaded()
	out := o.Create(forwardIn.Size()).(*Tensor)
	args := reluBackwardKernelArgs{
		In:     forwardIn.(*Tensor).ptr,
		Backin: backIn.(*Tensor).ptr,
		Out:    out.ptr,
		Count:  int32(forwardIn.NumElement()),
	}

	globalSize := [3]uint32{uint32(forwardIn.NumElement()), 1, 1}
	localSize := [3]uint16{64, 1, 1}

	o.timerStart()
	if o.Arch == arch.CDNA3 {
		o.launchReluBackwardCDNA3(globalSize, localSize,
			forwardIn.(*Tensor).ptr, backIn.(*Tensor).ptr,
			out.ptr, int32(forwardIn.NumElement()))
	} else {
		o.driver.LaunchKernel(o.ctx, o.reluBackwardKernel,
			globalSize, localSize, &args)
	}
	o.timerEnd("ReluBackward")

	if o.verification {
		cpuForwardIn := o.gpuTensorToCPUTensor(forwardIn)
		cpuBackIn := o.gpuTensorToCPUTensor(backIn)
		cpuOut := o.cpuOperator.ReluBackward(cpuForwardIn, cpuBackIn)
		o.tensorMustMatch(cpuOut, out)
		fmt.Println("ReluBackward verified.")
	}

	return out
}
