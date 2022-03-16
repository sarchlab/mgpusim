package mccl

import (
	"gitlab.com/akita/mem/v3/mem"
	"gitlab.com/akita/mgpusim/v2/driver"
)

// BroadcastRing broadcast data from the root to other GPUs.
func BroadcastRing(
	d *driver.Driver,
	comms []*Communicator,
	root int,
	data []driver.Ptr,
	dataSize int,
) {
	numGPU := len(comms)

	cmdQs := make([]*driver.CommandQueue, numGPU)
	for i := 0; i < numGPU; i++ {
		cmdQ := d.CreateCommandQueue(comms[i].Ctx)
		cmdQs[i] = cmdQ
	}

	chunkSize := int(4 * mem.KB)
	chunkNum := (dataSize-1)/chunkSize + 1

	distance := computeGPUDist(comms, root)

	for step := 0; step < (chunkNum + (numGPU - 2)); step++ {
		for i := 0; i < numGPU; i++ {
			if distance[i] == numGPU-1 {
				//last gpu should do nothing
				continue
			}

			srcChunkNum := step - distance[i]
			if srcChunkNum < 0 || srcChunkNum >= chunkNum {
				//do nothing when there is no data to send
				continue
			}

			src := data[i]
			src += driver.Ptr(4 * (srcChunkNum * chunkSize))
			dst := data[(i+1)%numGPU]
			dst += driver.Ptr(4 * (srcChunkNum * chunkSize))
			sizeToPush := min(chunkSize, dataSize-srcChunkNum*chunkSize)

			numThread := 1024
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

		for i := 0; i < numGPU; i++ {
			d.DrainCommandQueue(cmdQs[i])
		}
	}
}
