package mccl

import "gitlab.com/akita/mgpusim/v2/driver"

// AllReduceRing performs AllReduce average operation.
func AllReduceRing(
	d *driver.Driver,
	comms []*Communicator,
	data []driver.GPUPtr,
	dataSize int,
	bufs []driver.GPUPtr,
	sizePerBuf int,
) {
	numGPU := len(comms)
	cmdQs := createCommandQueues(d, comms)
	bigSteps := (dataSize-1)/sizePerBuf + 1
	numThread := 1024

	for j := 0; j < bigSteps; j++ {
		dataOffset := j * sizePerBuf
		currBufSize := min(sizePerBuf, dataSize-j*sizePerBuf)
		eachGPUDataSize := currBufSize / numGPU
		if eachGPUDataSize*numGPU < currBufSize {
			eachGPUDataSize++
		}

		// Step 1: Perform k-1 push and k-1 reduce.
		for step := 0; step < numGPU-1; step++ {
			allReducePushToBuffer(
				d, comms, cmdQs, data, bufs, step, numGPU, numThread,
				uint64(currBufSize),
				uint64(eachGPUDataSize),
				uint64(dataOffset),
			)
			for i := 0; i < numGPU; i++ {
				d.DrainCommandQueue(cmdQs[i])
			}

			allReduceReduce(
				d, comms, cmdQs, data, bufs, step, numGPU, numThread,
				uint64(currBufSize),
				uint64(eachGPUDataSize),
				uint64(dataOffset),
			)
			for i := 0; i < numGPU; i++ {
				d.DrainCommandQueue(cmdQs[i])
			}
		}

		//Step 2: k-1 steps push only. push to next gpu directly
		for step := 0; step < numGPU-1; step++ {
			allReducePushToGPU(
				d, comms, cmdQs, data, step, numGPU, numThread,
				uint64(currBufSize),
				uint64(eachGPUDataSize),
				uint64(dataOffset),
			)
			for i := 0; i < numGPU; i++ {
				d.DrainCommandQueue(cmdQs[i])
			}
		}
	}
}

func createCommandQueues(
	d *driver.Driver,
	comms []*Communicator,
) []*driver.CommandQueue {
	numGPU := len(comms)
	cmdQs := make([]*driver.CommandQueue, numGPU)

	for i := 0; i < numGPU; i++ {
		cmdQ := d.CreateCommandQueue(comms[i].Ctx)
		cmdQs[i] = cmdQ
	}

	return cmdQs
}

func allReducePushToBuffer(
	d *driver.Driver,
	comms []*Communicator,
	cmdQs []*driver.CommandQueue,
	data, bufs []driver.GPUPtr,
	step, numGPU, numThread int,
	currBufSize, pushSize, offset uint64,
) {
	for i := 0; i < numGPU; i++ {
		chunkIndex := uint64((i + numGPU - step) % numGPU)
		src := data[i]
		src += driver.GPUPtr(4 * (offset + chunkIndex*pushSize))
		dst := bufs[(i+1)%numGPU]
		dst += driver.GPUPtr(4 * chunkIndex * pushSize)

		sizeToPush := minUint64(pushSize, currBufSize-chunkIndex*pushSize)
		if sizeToPush < 0 {
			sizeToPush = 0
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
	d *driver.Driver,
	comms []*Communicator,
	cmdQs []*driver.CommandQueue,
	data, bufs []driver.GPUPtr,
	step, numGPU, numThread int,
	currBufSize, pushSize, offset uint64,
) {
	for i := 0; i < numGPU; i++ {
		chunkIndex := uint64((i + numGPU - (step + 1)) % numGPU)

		store := data[i]
		store += driver.GPUPtr(4 * (offset + chunkIndex*pushSize))
		buf := bufs[i]
		buf += driver.GPUPtr(4 * (chunkIndex * pushSize))

		sizeToPush := minUint64(pushSize, currBufSize-chunkIndex*pushSize)
		if sizeToPush < 0 {
			sizeToPush = 0
		}
		var lastReduce uint32 = 0
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
	d *driver.Driver,
	comms []*Communicator,
	cmdQs []*driver.CommandQueue,
	data []driver.GPUPtr,
	step, numGPU, numThread int,
	currBufSize, pushSize, offset uint64,
) {
	for i := 0; i < numGPU; i++ {
		chunkIndex := uint64((i + 1 + numGPU - step) % numGPU)

		src := data[i]
		src += driver.GPUPtr(4 * (offset + chunkIndex*pushSize))
		dst := data[(i+1)%numGPU]
		dst += driver.GPUPtr(4 * (offset + chunkIndex*pushSize))

		sizeToPush := minUint64(pushSize, currBufSize-chunkIndex*pushSize)
		if sizeToPush < 0 {
			sizeToPush = 0
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
