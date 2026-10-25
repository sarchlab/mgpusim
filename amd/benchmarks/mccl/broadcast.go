package mccl

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// BroadcastRing broadcast data from the root to other GPUs.
func BroadcastRing(
	d *driver.Runtime,
	comms []*Communicator,
	root int,
	data []driver.Ptr,
	dataSize int,
) *Collective {
	return &Collective{
		Kind:   "broadcast",
		Comms:  copyCommunicators(comms),
		Queues: createCommandQueues(d, comms),
		Root:   root,
		Data:   append([]driver.Ptr(nil), data...),
		Size:   dataSize,
	}
}

func (c *Collective) broadcast(d *driver.Runtime) {
	comms, data, dataSize, root, cmdQs := c.communicators(), c.Data, c.Size, c.Root, c.Queues
	numGPU := len(comms)
	chunkSize := int(4 * mem.KB)
	chunkNum := (dataSize-1)/chunkSize + 1

	distance := computeGPUDist(comms, root)

	if c.Stage >= chunkNum+numGPU-2 {
		c.Ended = true
		return
	}
	step := c.Stage
	{
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
	}
	c.Stage++
}
