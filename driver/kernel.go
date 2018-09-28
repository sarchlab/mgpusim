package driver

import (
	"encoding/binary"
	"log"

	"reflect"

	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

// A LaunchKernelEvent is a kernel even with an assigned time to run
//type LaunchKernelEvent struct {
//	*akita.EventBase
//
//	CO         *insts.HsaCo
//	GPU        akita.Component
//	Storage    *mem.Storage
//	GridSize   [3]uint32
//	WgSize     [3]uint16
//	KernelArgs interface{}
//}
//
//func (d *Driver) ScheduleKernelLaunching(
//	t akita.VTimeInSec,
//
//	co *insts.HsaCo,
//	gpu akita.Component,
//	storage *mem.Storage,
//	gridSize [3]uint32,
//	wgSize [3]uint16,
//	kernelArgs interface{},
//) {
//	k := new(LaunchKernelEvent)
//	k.EventBase = akita.NewEventBase(t, d)
//	d.engine.Schedule(k)
//
//	k.CO = co
//	k.GPU = gpu
//	k.Storage = storage
//	k.GridSize = gridSize
//	k.WgSize = wgSize
//	k.KernelArgs = kernelArgs
//}
//
//func (d *Driver) HandleLaunchKernelEvent(k *LaunchKernelEvent) error {
//	dCoData := d.AllocateMemory(k.Storage, uint64(len(k.CO.Data)))
//	d.MemoryCopyHostToDevice(dCoData, k.CO.Data, k.GPU)
//
//	dKernArgData := d.AllocateMemory(k.Storage, uint64(binary.Size(k.KernelArgs)))
//	d.MemoryCopyHostToDevice(dKernArgData, k.KernelArgs, k.GPU)
//
//	req := kernels.NewLaunchKernelReq()
//	req.HsaCo = k.CO
//	req.Packet = new(kernels.HsaKernelDispatchPacket)
//	req.Packet.GridSizeX = k.GridSize[0]
//	req.Packet.GridSizeY = k.GridSize[1]
//	req.Packet.GridSizeZ = k.GridSize[2]
//	req.Packet.WorkgroupSizeX = k.WgSize[0]
//	req.Packet.WorkgroupSizeY = k.WgSize[1]
//	req.Packet.WorkgroupSizeZ = k.WgSize[2]
//	req.Packet.KernelObject = uint64(dCoData)
//	req.Packet.KernargAddress = uint64(dKernArgData)
//
//	dPacket := d.AllocateMemory(k.Storage, uint64(binary.Size(req.Packet)))
//	d.MemoryCopyHostToDevice(dPacket, req.Packet, k.GPU)
//
//	req.PacketAddress = uint64(dPacket)
//	req.SetSrc(d.ToGPUs)
//	req.SetDst(k.GPU.ToDrive)
//	req.SetSendTime(0) // FIXME: The time need to be retrieved from the engine
//	err := d.ToGPUs.Send(req)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	return nil
//}

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

func (d *Driver) LaunchKernel(
	co *insts.HsaCo,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dCoData := d.copyInstructionsToGPU(co)
	dKernArgData := d.copyKernArgsToGPU(co, kernelArgs)
	packet, dPacket := d.createAQLPacket(gridSize, wgSize, dCoData, dKernArgData)
	d.runKernel(co, packet, dPacket)
	d.finalFlush()
}

func (d *Driver) copyKernArgsToGPU(co *insts.HsaCo, kernelArgs interface{}) GPUPtr {
	d.updateLDSPointers(co, kernelArgs)
	dKernArgData := d.AllocateMemoryWithAlignment(
		uint64(binary.Size(kernelArgs)), 4096)
	d.MemoryCopyHostToDevice(dKernArgData, kernelArgs)
	return dKernArgData
}

func (d *Driver) copyInstructionsToGPU(
	co *insts.HsaCo,
) GPUPtr {
	dCoData := d.AllocateMemoryWithAlignment(uint64(len(co.Data)), 4096)
	d.MemoryCopyHostToDevice(dCoData, co.Data)
	return dCoData
}

func (d *Driver) createAQLPacket(
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
	dPacket := d.AllocateMemoryWithAlignment(uint64(binary.Size(packet)), 4096)
	d.MemoryCopyHostToDevice(dPacket, packet)
	return packet, dPacket
}

func (d *Driver) runKernel(
	co *insts.HsaCo,
	packet *kernels.HsaKernelDispatchPacket,
	dPacket GPUPtr,
) {
	gpu := d.gpus[d.usingGPU]
	now := d.engine.CurrentTime() + 1e-8

	req := gcn3.NewLaunchKernelReq(now, d.ToGPUs, gpu.ToDriver)
	req.HsaCo = co
	req.Packet = packet
	startTime := now
	req.PacketAddress = uint64(dPacket)
	err := d.ToGPUs.Send(req)
	if err != nil {
		log.Panic(err)
	}
	d.kernelLaunchingStartTime[req.ID] = startTime
	d.engine.Run()
	//endTime := d.engine.CurrentTime()
	//fmt.Printf("Kernel: [%.012f - %.012f]\n", startTime, endTime)
	//return endTime
}

func (d *Driver) finalFlush() {
	gpu := d.gpus[d.usingGPU]
	now := d.engine.CurrentTime() + 1e-8
	flushCommand := gcn3.NewFlushCommand(now, d.ToGPUs, gpu.ToDriver)
	err := d.ToGPUs.Send(flushCommand)
	if err != nil {
		log.Panic(err)
	}
	d.engine.Run()
}
