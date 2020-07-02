package layers

import (
	"log"
	"math"

	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// MaxPoolingLayer Represent a MaxPooling Layer.
type MaxPoolingLayer struct {
	kernelSize                 [2]int //[H, W]
	stride                     [2]int //[H, W]
	padding                    [2]int //[H, W]
	B, C, Hin, Win, Hout, Wout int

	GPUDriver *driver.Driver
	GPUCtx    *driver.Context

	verifyForward  bool
	verifyBackward bool
	//cpuLayer       *layers.FullyConnectedLayer

	forwardKernel  *insts.HsaCo
	backwardKernel *insts.HsaCo

	forwardMask driver.GPUPtr //Record the indices of max element. Used in Backward propagation.
}

// KernelArgsForward defines forward kernel arguments
type KernelArgsForward struct {
	NumThreads uint64
	Bottom     driver.GPUPtr
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
	Top        driver.GPUPtr
	Mask       driver.GPUPtr //Record the indices of max element. Used in Backward propagation.

	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// KernelArgsBackward defines forward kernel arguments
type KernelArgsBackward struct {
	NumThreads uint64
	Top        driver.GPUPtr
	Mask       driver.GPUPtr
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
	Bottom     driver.GPUPtr

	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// NewMaxPoolingLayer creates a new max pooling layer.
func NewMaxPoolingLayer(
	stride [2]int,
	padding [2]int,
	kernelSize [2]int,
	driver *driver.Driver,
	ctx *driver.Context,
) *MaxPoolingLayer {
	m := &MaxPoolingLayer{
		GPUDriver:  driver,
		GPUCtx:     ctx,
		kernelSize: kernelSize,
		padding:    padding,
		stride:     stride,
	}
	kernelBytes := _escFSMustByte(false, "/maxpooling.hsaco")
	m.forwardKernel = kernels.LoadProgramFromMemory(
		kernelBytes, "MaxPoolForward")
	if m.forwardKernel == nil {
		panic("fail to load maxpooling forward kernel")
	}

	m.backwardKernel = kernels.LoadProgramFromMemory(
		kernelBytes, "MaxPoolBackward")
	if m.backwardKernel == nil {
		panic("fail to load maxpooling backward kernel")
	}
	return m
}

// EnableVerification runs a CPU pass for every forward and backward propagation
// though the max pooling layer to make sure the simulator is correct.
func (m *MaxPoolingLayer) EnableVerification() {
	m.verifyForward = true
	m.verifyBackward = true
}

func (m *MaxPoolingLayer) saveMask(input *Tensor) {
	if m.forwardMask != 0 {
		m.GPUDriver.FreeMemory(m.GPUCtx, m.forwardMask)
	}

	numElement := input.Size()[0] * input.Size()[1] * input.Size()[2] * input.Size()[3]

	m.forwardMask = m.GPUDriver.AllocateMemory(m.GPUCtx,
		uint64(numElement*4))

	temp := make([]uint32, numElement)
	m.GPUDriver.MemCopyD2H(m.GPUCtx, temp, input.ptr)
	m.GPUDriver.MemCopyH2D(m.GPUCtx, m.forwardMask, temp)
}

//MinInt Find Min of two Ints.
func MinInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

//MaxInt Find Max of two Ints.
func MaxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// CPUMaxpooling returns output of maxpooling layer.
func (m *MaxPoolingLayer) CPUMaxpooling(input []float64) []float32 {
	outputLen := m.B * m.C * m.Hout * m.Wout
	StrideH := m.stride[0]
	StrideW := m.stride[1]
	PadH := m.padding[0]
	PadW := m.padding[1]
	KernelH := m.kernelSize[0]
	KernelW := m.kernelSize[1]
	cpuOutput := make([]float32, outputLen)
	hin := m.Hin
	hout := m.Hout
	win := m.Win
	wout := m.Wout
	C := m.C

	for i := 0; i < outputLen; i++ {
		pw := i % wout
		ph := (i / wout) % hout
		c := (i / wout / hout) % C
		n := i / wout / hout / C
		hStart := ph*StrideH - PadH
		wStart := pw*StrideW - PadW
		hEnd := MinInt(hStart+KernelH, hin)
		wEnd := MinInt(wStart+KernelW, win)
		hStart = MaxInt(hStart, 0)
		wStart = MaxInt(wStart, 0)

		maxVal := float64(-math.MaxFloat32)
		offset := (n*C + c) * hin * win
		for h := hStart; h < hEnd; h++ {
			for w := wStart; w < wEnd; w++ {
				if input[h*win+w+offset] > maxVal {
					maxVal = input[h*win+w+offset]
				}
			}
		}

		cpuOutput[i] = float32(maxVal)
	}

	return cpuOutput
}

// Forward performs the forward propagation algorithm.
//nolint:funlen
func (m *MaxPoolingLayer) Forward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*Tensor)
	B := inputT.Size()[0]
	C := inputT.Size()[1]
	Hin := inputT.Size()[2]
	Win := inputT.Size()[3]
	ks := m.kernelSize
	stride := m.stride
	padding := m.padding
	Hout := (Hin+2*padding[0]-ks[0])/stride[0] + 1
	Wout := (Win+2*padding[1]-ks[1])/stride[1] + 1
	m.B = B
	m.C = C
	m.Hin = Hin
	m.Hout = Hout
	m.Win = Win
	m.Wout = Wout
	output := &Tensor{
		driver: m.GPUDriver,
		ctx:    m.GPUCtx,
		size:   []int{B, C, Hout, Wout},
		ptr:    m.GPUDriver.AllocateMemory(m.GPUCtx, uint64(B*C*Hout*Wout*4)),
	}
	mask := &Tensor{
		driver: m.GPUDriver,
		ctx:    m.GPUCtx,
		size:   []int{B, C, Hout, Wout},
		ptr:    m.GPUDriver.AllocateMemory(m.GPUCtx, uint64(B*C*Hout*Wout*4)),
	}
	kernArg := KernelArgsForward{
		uint64(B * C * Hout * Wout), input.ptr,
		int32(B), int32(C), int32(Hin), int32(Win),
		int32(Hout), int32(Wout),
		int32(ks[0]), int32(ks[1]),
		int32(stride[0]), int32(stride[1]),
		int32(padding[0]), int32(padding[1]),
		output.ptr,
		mask.ptr,
		0, 0, 0,
	}
	m.GPUDriver.LaunchKernel(
		m.GPUCtx,
		m.forwardKernel,
		[3]uint32{uint32(B * C * Hout * Wout), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg,
	)

	if m.verifyForward {
		cpuOutput := m.CPUMaxpooling(input.Vector())
		m.verifyForwardPass(cpuOutput, output)
	}

	m.saveMask(mask)
	return output
}

func (m *MaxPoolingLayer) verifyForwardPass(cpu []float32, output *Tensor) {
	misMatch := false
	gpu := output.Vector()

	for i := 0; i < len(cpu); i++ {
		if cpu[i] != float32(gpu[i]) {
			log.Printf("Mismatch at %d, expected %f, but get %f.",
				i, cpu[i], gpu[i])
			misMatch = true
		}
	}

	if misMatch {
		panic("forward pass verification failed")
	}
}

// Backward performs the backward propagation operation.
func (m *MaxPoolingLayer) Backward(inputT tensor.Tensor) tensor.Tensor {
	input := inputT.(*Tensor)
	ks := m.kernelSize
	stride := m.stride
	padding := m.padding
	B := m.B
	C := m.C
	Hin := m.Hin
	Hout := m.Hout
	Win := m.Win
	Wout := m.Wout
	output := &Tensor{
		driver: m.GPUDriver,
		ctx:    m.GPUCtx,
		size:   []int{B, C, Hin, Win},
		ptr:    m.GPUDriver.AllocateMemory(m.GPUCtx, uint64(B*C*Hin*Win*4)),
	}

	kernArg := KernelArgsBackward{
		uint64(B * C * Hin * Win), input.ptr, m.forwardMask,
		int32(B), int32(C), int32(Hin), int32(Win),
		int32(Hout), int32(Wout),
		int32(ks[0]), int32(ks[1]),
		int32(stride[0]), int32(stride[1]),
		int32(padding[0]), int32(padding[1]),
		output.ptr,
		0, 0, 0,
	}

	m.GPUDriver.LaunchKernel(
		m.GPUCtx,
		m.backwardKernel,
		[3]uint32{uint32(B * C * Hin * Win), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg,
	)

	if m.verifyBackward {
		m.verifyBackPass(input, output)
	}

	return output
}

func (m *MaxPoolingLayer) verifyBackPass(input *Tensor, output *Tensor) {
	inputV := input.Vector()
	outputV := output.Vector()
	mask := make([]uint32, input.Size()[0]*input.Size()[1]*input.Size()[2]*input.Size()[3])
	m.GPUDriver.MemCopyD2H(m.GPUCtx, mask, m.forwardMask)
	count := 0
	var i uint32 = 0
	for i = 0; int(i) < len(outputV); i++ {
		if i+1 == mask[count] {
			if inputV[count] != outputV[i] {
				log.Panicf("Mismatch at %d, expected %f, but get %f.",
					i, inputV[count], outputV[i])
			}
		}
	}
}
