package driver

import (
	"encoding/binary"
	"reflect"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// EnqueueLaunchKernel schedules kernel to be launched later
func (d *Driver) EnqueueLaunchKernel(
	queue *CommandQueue,
	co *insts.HsaCo,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dev := d.devices[queue.GPUID]

	if dev.Type == internal.DeviceTypeUnifiedGPU {
		d.enqueueLaunchUnifiedKernel(queue, co, gridSize, wgSize, kernelArgs)
	} else {
		dCoData, dKernArgData, dPacket := d.allocateGPUMemory(queue.Context, co)

		packet := d.createAQLPacket(gridSize, wgSize, dCoData, dKernArgData)
		newKernelArgs := d.prepareLocalMemory(co, kernelArgs, packet)

		d.EnqueueMemCopyH2D(queue, dCoData, co.Data)
		d.EnqueueMemCopyH2D(queue, dKernArgData, newKernelArgs)
		d.EnqueueMemCopyH2D(queue, dPacket, packet)

		d.enqueueLaunchKernelCommand(queue, co, packet, dPacket)
	}
}

func (d *Driver) allocateGPUMemory(
	ctx *Context,
	co *insts.HsaCo,
) (dCoData, dKernArgData, dPacket Ptr) {
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
) (newKernelArgs interface{}) {
	newKernelArgs = reflect.New(reflect.TypeOf(kernelArgs).Elem()).Interface()
	reflect.ValueOf(newKernelArgs).Elem().
		Set(reflect.ValueOf(kernelArgs).Elem())

	ldsSize := co.WGGroupSegmentByteSize

	if reflect.TypeOf(newKernelArgs).Kind() == reflect.Slice {
		// From server, do nothing
	} else {
		kernArgStruct := reflect.ValueOf(newKernelArgs).Elem()
		for i := 0; i < kernArgStruct.NumField(); i++ {
			arg := kernArgStruct.Field(i).Interface()

			switch ldsPtr := arg.(type) {
			case LocalPtr:
				kernArgStruct.Field(i).SetUint(uint64(ldsSize))
				ldsSize += uint32(ldsPtr)
			}
		}
	}

	packet.GroupSegmentSize = ldsSize

	return newKernelArgs
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
	dCoData Ptr,
	dKernArgData Ptr,
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
	dPacket Ptr,
) {
	cmd := &LaunchKernelCommand{
		ID:         sim.GetIDGenerator().Generate(),
		CodeObject: co,
		DPacket:    dPacket,
		Packet:     packet,
	}
	d.Enqueue(queue, cmd)
}

func (d *Driver) enqueueLaunchUnifiedKernelCommand(
	queue *CommandQueue,
	co *insts.HsaCo,
	packet []*kernels.HsaKernelDispatchPacket,
	dPacket []Ptr,
) {
	cmd := &LaunchUnifiedMultiGPUKernelCommand{
		ID:           sim.GetIDGenerator().Generate(),
		CodeObject:   co,
		DPacketArray: dPacket,
		PacketArray:  packet,
	}
	d.Enqueue(queue, cmd)
}
func (d *Driver) enqueueLaunchUnifiedKernel(
	queue *CommandQueue,
	co *insts.HsaCo,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dev := d.devices[queue.GPUID]
	initGPUID := queue.Context.currentGPUID
	queueArray := make([]*CommandQueue, len(dev.UnifiedGPUIDs)+1)
	dCoDataArray := make([]Ptr, len(dev.UnifiedGPUIDs)+1)
	dKernArgDataArray := make([]Ptr, len(dev.UnifiedGPUIDs)+1)
	dPacketArray := make([]Ptr, len(dev.UnifiedGPUIDs)+1)
	packetArray := make([]*kernels.HsaKernelDispatchPacket, len(dev.UnifiedGPUIDs)+1)
	// fmt.Printf("# of GPUs : %v \n", len(dev.UnifiedGPUIDs))

	for i, gpuID := range dev.UnifiedGPUIDs {
		queueArray[i] = queue
		queueArray[i].Context.currentGPUID = gpuID
		dCoData, dKernArgData, dPacket := d.allocateGPUMemory(queue.Context, co)

		packet := d.createAQLPacket(gridSize, wgSize, dCoData, dKernArgData)
		newKernelArgs := d.prepareLocalMemory(co, kernelArgs, packet)

		d.EnqueueMemCopyH2D(queue, dCoData, co.Data)
		d.EnqueueMemCopyH2D(queue, dKernArgData, newKernelArgs)
		d.EnqueueMemCopyH2D(queue, dPacket, packet)

		dCoDataArray[i] = dCoData
		dKernArgDataArray[i] = dKernArgData
		dPacketArray[i] = dPacket
		packetArray[i] = packet
		// fmt.Printf("packetArray: %v \n", packetArray[i])
	}

	queue.Context.currentGPUID = initGPUID
	d.enqueueLaunchUnifiedKernelCommand(queue, co, packetArray, dPacketArray)
}
