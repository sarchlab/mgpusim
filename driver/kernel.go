package driver

import (
	"encoding/binary"
	"reflect"

	"github.com/rs/xid"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

// EnqueueLaunchKernel schedules kernel to be launched later
func (d *Driver) EnqueueLaunchKernel(
	queue *CommandQueue,
	co *insts.HsaCo,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dCoData := d.enqueueCopyInstructionsToGPU(queue, co)
	dKernArgData := d.enqueueCopyKernArgsToGPU(queue, co, kernelArgs)
	packet, dPacket := d.createAQLPacket(
		queue, gridSize, wgSize, dCoData, dKernArgData)
	d.enqueueLaunchKernelCommand(queue, co, packet, dPacket)
	d.enqueueFinalFlush(queue)
}

func (d *Driver) updateLDSPointers(co *insts.HsaCo, kernelArgs interface{}) {
	ldsSize := uint32(0)
	kernArgStruct := reflect.ValueOf(kernelArgs).Elem()
	for i := 0; i < kernArgStruct.NumField(); i++ {
		arg := kernArgStruct.Field(i).Interface()
		switch ldsPtr := arg.(type) {
		case LocalPtr:
			kernArgStruct.Field(i).SetUint(uint64(ldsSize))
			ldsSize += uint32(ldsPtr)
		}
	}
	co.WGGroupSegmentByteSize = ldsSize
}

// LaunchKernel is an easy way to run a kernel on the GCN3 simulator. It
// launches the kernel immediately.
func (d *Driver) LaunchKernel(
	ctx *Context,
	co *insts.HsaCo,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	queue := d.CreateCommandQueue(ctx)
	d.EnqueueLaunchKernel(queue, co, gridSize, wgSize, kernelArgs)
	d.DrainCommandQueue(queue)
}

func (d *Driver) enqueueCopyKernArgsToGPU(
	queue *CommandQueue,
	co *insts.HsaCo,
	kernelArgs interface{},
) GPUPtr {
	d.updateLDSPointers(co, kernelArgs)
	dKernArgData := d.AllocateMemoryWithAlignment(
		queue.Context, uint64(binary.Size(kernelArgs)), 4096)
	d.EnqueueMemCopyH2D(queue, dKernArgData, kernelArgs)
	return dKernArgData
}

func (d *Driver) enqueueCopyInstructionsToGPU(
	queue *CommandQueue,
	co *insts.HsaCo,
) GPUPtr {
	dCoData := d.AllocateMemoryWithAlignment(
		queue.Context,
		uint64(len(co.Data)), 4096)
	d.EnqueueMemCopyH2D(queue, dCoData, co.Data)
	return dCoData
}

func (d *Driver) createAQLPacket(
	queue *CommandQueue,
	gridSize [3]uint32,
	wgSize [3]uint16,
	dCoData GPUPtr,
	dKernArgData GPUPtr,
) (*kernels.HsaKernelDispatchPacket, GPUPtr) {
	packet := new(kernels.HsaKernelDispatchPacket)
	packet.GridSizeX = gridSize[0]
	packet.GridSizeY = gridSize[1]
	packet.GridSizeZ = gridSize[2]
	packet.WorkgroupSizeX = wgSize[0]
	packet.WorkgroupSizeY = wgSize[1]
	packet.WorkgroupSizeZ = wgSize[2]
	packet.KernelObject = uint64(dCoData)
	packet.KernargAddress = uint64(dKernArgData)
	dPacket := d.AllocateMemoryWithAlignment(
		queue.Context, uint64(binary.Size(packet)), 4096)
	d.EnqueueMemCopyH2D(queue, dPacket, packet)
	return packet, dPacket
}

func (d *Driver) enqueueLaunchKernelCommand(
	queue *CommandQueue,
	co *insts.HsaCo,
	packet *kernels.HsaKernelDispatchPacket,
	dPacket GPUPtr,
) {
	cmd := &LaunchKernelCommand{
		ID:         xid.New().String(),
		CodeObject: co,
		DPacket:    dPacket,
		Packet:     packet,
	}
	d.Enqueue(queue, cmd)
}

func (d *Driver) enqueueFinalFlush(queue *CommandQueue) {
	cmd := &FlushCommand{
		ID: xid.New().String(),
	}
	d.Enqueue(queue, cmd)
}
