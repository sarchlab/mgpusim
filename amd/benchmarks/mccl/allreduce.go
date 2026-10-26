package mccl

import (
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// AllReduceRing creates a resumable average reduction. Call Step only after
// the preceding global simulation batch has drained.
func AllReduceRing(
	d *gpuruntime.Runtime,
	comms []*Communicator,
	data []gpuruntime.Ptr,
	dataSize int,
	bufs []gpuruntime.Ptr,
	sizePerBuf int,
) *Collective {
	return &Collective{
		Kind:       "reduce",
		Comms:      copyCommunicators(comms),
		Queues:     createCommandQueues(d, comms),
		Data:       append([]gpuruntime.Ptr(nil), data...),
		Buffers:    append([]gpuruntime.Ptr(nil), bufs...),
		Size:       dataSize,
		BufferSize: sizePerBuf,
	}
}

func createCommandQueues(
	d *gpuruntime.Runtime,
	comms []*Communicator,
) []gpuruntime.CommandQueue {
	numGPU := len(comms)
	cmdQs := make([]gpuruntime.CommandQueue, numGPU)

	for i := 0; i < numGPU; i++ {
		d.SelectGPU(comms[i].Ctx, comms[i].GPUID)
		cmdQ := d.CreateCommandQueue(comms[i].Ctx)
		cmdQs[i] = cmdQ
	}

	return cmdQs
}

func allReducePushToBuffer(
	d *gpuruntime.Runtime,
	comms []*Communicator,
	cmdQs []gpuruntime.CommandQueue,
	data, bufs []gpuruntime.Ptr,
	step, numGPU, numThread int,
	currBufSize, pushSize, offset uint64,
) {
	for i := 0; i < numGPU; i++ {
		chunkIndex := uint64((i + numGPU - step) % numGPU)
		src := data[i]
		src += gpuruntime.Ptr(4 * (offset + chunkIndex*pushSize))
		dst := bufs[(i+1)%numGPU]
		dst += gpuruntime.Ptr(4 * chunkIndex * pushSize)

		sizeToPush := uint64(0)
		if chunkIndex*pushSize < currBufSize {
			sizeToPush = minUint64(pushSize, currBufSize-chunkIndex*pushSize)
		}

		d.SelectGPU(comms[i].Ctx, comms[i].GPUID)
		kernelArgs := &pushKernelArgs{
			Src:       src,
			Dst:       dst,
			Size:      uint32(sizeToPush),
			NumThread: uint32(numThread),
		}
		d.EnqueueLaunchKernel(
			cmdQs[i],
			coPush,
			[3]uint32{uint32(numThread), 1, 1},
			[3]uint16{64, 1, 1},
			kernelArgs,
		)
	}
}

func allReduceReduce(
	d *gpuruntime.Runtime,
	comms []*Communicator,
	cmdQs []gpuruntime.CommandQueue,
	data, bufs []gpuruntime.Ptr,
	step, numGPU, numThread int,
	currBufSize, pushSize, offset uint64,
) {
	for i := 0; i < numGPU; i++ {
		chunkIndex := uint64((i + numGPU - (step + 1)) % numGPU)

		store := data[i]
		store += gpuruntime.Ptr(4 * (offset + chunkIndex*pushSize))
		buf := bufs[i]
		buf += gpuruntime.Ptr(4 * (chunkIndex * pushSize))

		sizeToPush := uint64(0)
		if chunkIndex*pushSize < currBufSize {
			sizeToPush = minUint64(pushSize, currBufSize-chunkIndex*pushSize)
		}

		var lastReduce uint32
		if step == numGPU-2 {
			//last reduce
			lastReduce = 1
		}

		d.SelectGPU(comms[i].Ctx, comms[i].GPUID)
		kernelArgs := &allReduceReduceKernelArgs{
			Buf:       buf,
			Store:     store,
			Size:      uint32(sizeToPush),
			NumThread: uint32(numThread),
			GPUNum:    uint32(numGPU),
			Last:      lastReduce,
		}
		d.EnqueueLaunchKernel(
			cmdQs[i],
			coReduce,
			[3]uint32{uint32(numThread), 1, 1},
			[3]uint16{64, 1, 1},
			kernelArgs,
		)
	}
}

func allReducePushToGPU(
	d *gpuruntime.Runtime,
	comms []*Communicator,
	cmdQs []gpuruntime.CommandQueue,
	data []gpuruntime.Ptr,
	step, numGPU, numThread int,
	currBufSize, pushSize, offset uint64,
) {
	for i := 0; i < numGPU; i++ {
		chunkIndex := uint64((i + 1 + numGPU - step) % numGPU)

		src := data[i]
		src += gpuruntime.Ptr(4 * (offset + chunkIndex*pushSize))
		dst := data[(i+1)%numGPU]
		dst += gpuruntime.Ptr(4 * (offset + chunkIndex*pushSize))

		sizeToPush := uint64(0)
		if chunkIndex*pushSize < currBufSize {
			sizeToPush = minUint64(pushSize, currBufSize-chunkIndex*pushSize)
		}

		d.SelectGPU(comms[i].Ctx, comms[i].GPUID)
		kernelArgs := &pushKernelArgs{
			Src:       src,
			Dst:       dst,
			Size:      uint32(sizeToPush),
			NumThread: uint32(numThread),
		}
		d.EnqueueLaunchKernel(
			cmdQs[i],
			coPush,
			[3]uint32{uint32(numThread), 1, 1},
			[3]uint16{64, 1, 1},
			kernelArgs,
		)
	}
}
