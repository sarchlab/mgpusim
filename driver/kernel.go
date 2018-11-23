package driver

import (
	"encoding/binary"
	"log"

	"reflect"

	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
)

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

// LaunchKernel is an eaiser way to run a kernel on the GCN3 simulator. It
// launches the kernel immediately.
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
	req.PacketAddress = uint64(dPacket)

	sendErr := d.ToGPUs.Send(req)
	if sendErr != nil {
		log.Panic(sendErr)
	}

	d.InvokeHook(req, d, HookPosReqStart, nil)
	err := d.engine.Run()
	if err != nil {
		log.Panic()
	}
	//endTime := d.engine.CurrentTime()
	//fmt.Printf("Kernel: [%.012f - %.012f]\n", startTime, endTime)
	//return endTime
}

func (d *Driver) finalFlush() {
	gpu := d.gpus[d.usingGPU]
	now := d.engine.CurrentTime() + 1e-8
	flushCommand := gcn3.NewFlushCommand(now, d.ToGPUs, gpu.ToDriver)
	sendErr := d.ToGPUs.Send(flushCommand)
	if sendErr != nil {
		log.Panic(sendErr)
	}

	err := d.engine.Run()
	if err != nil {
		log.Panic(err)
	}

}
