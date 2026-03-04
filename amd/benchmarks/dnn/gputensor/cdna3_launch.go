package gputensor

import (
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

func (o *GPUOperator) launchRepeatCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	output, input driver.Ptr,
	inputLen, outLen uint32,
) {
	args := cdna3RepeatArgs{
		Output:          output,
		Input:           input,
		InputLen:        inputLen,
		OutLen:          outLen,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.repeatKernel,
		globalSize, localSize, &args)
}

func (o *GPUOperator) launchTransposeCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *transposeKernelArgs,
) {
	cdna3Args := cdna3TransposeKernelArgs{
		In:              args.In,
		Out:             args.Out,
		InSize:          args.InSize,
		OutSize:         args.OutSize,
		Order:           args.Order,
		InIndexBuf:      args.InIndexBuf,
		OutIndexBuf:     args.OutIndexBuf,
		Dim:             args.Dim,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.transposeKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchRotateCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *rotateKernelArgs,
) {
	cdna3Args := cdna3RotateKernelArgs{
		In:              args.In,
		Out:             args.Out,
		InSize:          args.InSize,
		OutSize:         args.OutSize,
		InIndexBuf:      args.InIndexBuf,
		OutIndexBuf:     args.OutIndexBuf,
		Dim:             args.Dim,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.rotateKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchDilateCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *dilateKernelArgs,
) {
	cdna3Args := cdna3DilateKernelArgs{
		In:              args.In,
		Out:             args.Out,
		InSize:          args.InSize,
		OutSize:         args.OutSize,
		Dilate:          args.Dilate,
		InIndexBuf:      args.InIndexBuf,
		OutIndexBuf:     args.OutIndexBuf,
		Dim:             args.Dim,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.dilateKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchSumOneAxisCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *sumOneAxisKernelArgs,
) {
	cdna3Args := cdna3SumOneAxisKernelArgs{
		In:              args.In,
		Out:             args.Out,
		InSize:          args.InSize,
		OutSize:         args.OutSize,
		InDim:           args.InDim,
		Axis:            args.Axis,
		InIndexBuf:      args.InIndexBuf,
		OutIndexBuf:     args.OutIndexBuf,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.sumKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchGemmCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *gemmKernArgs,
) {
	cdna3Args := cdna3GemmKernArgs{
		M:               args.M,
		N:               args.N,
		K:               args.K,
		Alpha:           args.Alpha,
		Beta:            args.Beta,
		A:               args.A,
		B:               args.B,
		C:               args.C,
		D:               args.D,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.gemmKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchIm2ColCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *im2ColKernelArg,
) {
	cdna3Args := cdna3Im2ColKernelArg{
		Input:           args.Input,
		Output:          args.Output,
		InputDim:        args.InputDim,
		MaskDim:         args.MaskDim,
		Stride:          args.Stride,
		PadArg:          args.Pad,
		Dilation:        args.Dilation,
		Channel:         args.Channel,
		Batch:           args.Batch,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.im2ColKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchMaxPoolFwdCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *maxPoolingForwardKernelArgs,
) {
	cdna3Args := cdna3MaxPoolingForwardKernelArgs{
		Nthreads:        args.NThreads,
		BottomData:      args.BottomData,
		Num:             args.Num,
		Channels:        args.Channels,
		Height:          args.Height,
		Width:           args.Width,
		PooledHeight:    args.PooledH,
		PooledWidth:     args.PooledW,
		KernelH:         args.KernelH,
		KernelW:         args.KernelW,
		StrideH:         args.StrideH,
		StrideW:         args.StrideW,
		PadH:            args.PadH,
		PadW:            args.PadW,
		TopData:         args.TopData,
		MaskData:        args.MaskData,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.maxPoolingForwardKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchMaxPoolBwdCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *maxPoolingBackwardKernelArgs,
) {
	cdna3Args := cdna3MaxPoolingBackwardKernelArgs{
		Nthreads:        args.NThreads,
		TopDiff:         args.TopDiff,
		TopMask:         args.TopMask,
		Num:             args.Num,
		Channels:        args.Channels,
		Height:          args.Height,
		Width:           args.Width,
		PooledHeight:    args.PooledHeight,
		PooledWidth:     args.PooledWidth,
		KernelH:         args.KernelH,
		KernelW:         args.KernelW,
		StrideH:         args.StrideH,
		StrideW:         args.StrideW,
		PadH:            args.PadH,
		PadW:            args.PadW,
		BottomDiff:      args.BottomDiff,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.maxPoolingBackwardKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchAvgPoolFwdCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *AvgPoolingKernelArgsForward,
) {
	cdna3Args := cdna3AvgPoolingForwardKernelArgs{
		Nthreads:        int32(args.NumThreads),
		BottomData:      args.Bottom,
		Num:             args.N,
		Channels:        args.C,
		Height:          args.H,
		Width:           args.W,
		PooledHeight:    args.PooledH,
		PooledWidth:     args.PooledW,
		KernelH:         args.KernelH,
		KernelW:         args.KernelW,
		StrideH:         args.StrideH,
		StrideW:         args.StrideW,
		PadH:            args.PadH,
		PadW:            args.PadW,
		TopData:         args.Top,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.avgPoolingForwardKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchAvgPoolBwdCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *AvgPoolingKernelArgsBackward,
) {
	cdna3Args := cdna3AvgPoolingBackwardKernelArgs{
		Nthreads:        int32(args.NumThreads),
		TopDiff:         args.Top,
		Num:             args.N,
		Channels:        args.C,
		Height:          args.H,
		Width:           args.W,
		PooledHeight:    args.PooledH,
		PooledWidth:     args.PooledW,
		KernelH:         args.KernelH,
		KernelW:         args.KernelW,
		StrideH:         args.StrideH,
		StrideW:         args.StrideW,
		PadH:            args.PadH,
		PadW:            args.PadW,
		BottomDiff:      args.Bottom,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.avgPoolingBackwardKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchSoftmaxExpCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	input, output driver.Ptr,
	n int32,
) {
	args := cdna3SoftmaxExpArgs{
		Input:           input,
		Output:          output,
		N:               n,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.softmaxExpKernel,
		globalSize, localSize, &args)
}

func (o *GPUOperator) launchSoftmaxDivCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	expInput, output, denominator driver.Ptr,
	numElement, batchSize int32,
) {
	args := cdna3SoftmaxDivArgs{
		ExpInput:        expInput,
		Out:             output,
		Denominator:     denominator,
		NumElement:      numElement,
		BatchSize:       batchSize,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.softmaxDivKernel,
		globalSize, localSize, &args)
}

func (o *GPUOperator) launchCrossEntropyDerivCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	kernel *insts.KernelCodeObject,
	args *crossEntropyDerivativeArgs,
) {
	cdna3Args := cdna3CrossEntropyDerivativeArgs{
		Output:          args.Output,
		Input:           args.Input,
		Label:           args.Label,
		BatchSize:       args.BatchSize,
		NumPerImage:     args.NumPerImage,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, kernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchElemWiseMulCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	out, in1, in2 driver.Ptr,
	n int32,
) {
	args := cdna3MulArgs{
		Out:             out,
		In1:             in1,
		In2:             in2,
		N:               n,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.elemWiseMulKernel,
		globalSize, localSize, &args)
}

func (o *GPUOperator) launchScaleAddCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *scaleAddKernArg,
) {
	cdna3Args := cdna3ScaleAddArgs{
		Out:             args.Out,
		In1:             args.In1,
		In2:             args.In2,
		Alpha:           args.Alpha,
		Beta:            args.Beta,
		N:               args.N,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.scaleAddKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchRmsPropCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *rmsPropKernArg,
) {
	cdna3Args := cdna3RmsPropArgs{
		Params:          args.Params,
		Gradients:       args.Gradients,
		SHistory:        args.SHistory,
		SmoothFactor:    args.SmoothFactor,
		LearningRate:    args.LearningRate,
		N:               args.N,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.rmsPropKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchAdamCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	args *adamKernArg,
) {
	cdna3Args := cdna3AdamArgs{
		Params:          args.Params,
		Gradients:       args.Gradients,
		SHistory:        args.SHistory,
		VHistory:        args.VHistory,
		SmoothFactor1:   args.SmoothFactor1,
		SmoothFactor2:   args.SmoothFactor2,
		LearningRate:    args.LearningRate,
		N:               args.N,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.adamKernel,
		globalSize, localSize, &cdna3Args)
}

func (o *GPUOperator) launchReluForwardCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	in, out driver.Ptr,
	count int32,
) {
	args := cdna3ReluForwardArgs{
		In:              in,
		Out:             out,
		Count:           count,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.reluForwardKernel,
		globalSize, localSize, &args)
}

func (o *GPUOperator) launchReluBackwardCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
	in, backIn, out driver.Ptr,
	count int32,
) {
	args := cdna3ReluBackwardArgs{
		In:              in,
		BackIn:          backIn,
		Out:             out,
		Count:           count,
		cdna3HiddenArgs: newCDNA3HiddenArgs(globalSize, localSize),
	}
	o.driver.LaunchKernel(o.ctx, o.reluBackwardKernel,
		globalSize, localSize, &args)
}
