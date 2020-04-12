package driver

import (
	"encoding/binary"
	"reflect"

	"github.com/rs/xid"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// EnqueueLaunchKernel schedules kernel to be launched later
func (d *Driver) EnqueueLaunchKernel(
	queue *CommandQueue,
	co *insts.HsaCo,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dCoData, dKernArgData, dPacket := d.allocateGPUMemory(queue.Context, co)

	packet := d.createAQLPacket(gridSize, wgSize, dCoData, dKernArgData)
	d.prepareLocalMemory(co, kernelArgs, packet)

	d.EnqueueMemCopyH2D(queue, dCoData, co.Data)
	d.EnqueueMemCopyH2D(queue, dKernArgData, kernelArgs)
	d.EnqueueMemCopyH2D(queue, dPacket, packet)

	d.enqueueLaunchKernelCommand(queue, co, packet, dPacket)
}

func (d *Driver) allocateGPUMemory(
	ctx *Context,
	co *insts.HsaCo,
) (dCoData, dKernArgData, dPacket GPUPtr) {
	dCoData = d.AllocateMemory(ctx, uint64(len(co.Data)))
	dKernArgData = d.AllocateMemory(ctx, co.KernargSegmentByteSize)

	packet := kernels.HsaKernelDispatchPacket{}
	dPacket = d.AllocateMemory(ctx, uint64(binary.Size(packet)))

	return dCoData, dKernArgData, dPacket
}

func (d *Driver) prepareLocalMemory(
	co *insts.HsaCo,
	kernelArgs interface{},
	packet *kernels.HsaKernelDispatchPacket,
) {
	ldsSize := co.WGGroupSegmentByteSize

	kernArgStruct := reflect.ValueOf(kernelArgs).Elem()
	for i := 0; i < kernArgStruct.NumField(); i++ {
		arg := kernArgStruct.Field(i).Interface()

		switch ldsPtr := arg.(type) {
		case LocalPtr:
			kernArgStruct.Field(i).SetUint(uint64(ldsSize))
			ldsSize += uint32(ldsPtr)
		}
	}

	packet.GroupSegmentSize = ldsSize
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

func (d *Driver) createAQLPacket(
	gridSize [3]uint32,
	wgSize [3]uint16,
	dCoData GPUPtr,
	dKernArgData GPUPtr,
) *kernels.HsaKernelDispatchPacket {
	packet := new(kernels.HsaKernelDispatchPacket)
	packet.GridSizeX = gridSize[0]
	packet.GridSizeY = gridSize[1]
	packet.GridSizeZ = gridSize[2]
	packet.WorkgroupSizeX = wgSize[0]
	packet.WorkgroupSizeY = wgSize[1]
	packet.WorkgroupSizeZ = wgSize[2]
	packet.KernelObject = uint64(dCoData)
	packet.KernargAddress = uint64(dKernArgData)
	return packet
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
