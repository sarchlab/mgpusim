package driver

import (
	"encoding/binary"
	"log"

	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/mem"
	"fmt"
)

// A LaunchKernelEvent is a kernel even with an assigned time to run
type LaunchKernelEvent struct {
	*core.EventBase

	CO         *insts.HsaCo
	GPU        core.Component
	Storage    *mem.Storage
	GridSize   [3]uint32
	WgSize     [3]uint16
	KernelArgs interface{}
}

func (d *Driver) ScheduleKernelLaunching(
	t core.VTimeInSec,

	co *insts.HsaCo,
	gpu core.Component,
	storage *mem.Storage,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	k := new(LaunchKernelEvent)
	k.EventBase = core.NewEventBase(t, d)
	d.engine.Schedule(k)

	k.CO = co
	k.GPU = gpu
	k.Storage = storage
	k.GridSize = gridSize
	k.WgSize = wgSize
	k.KernelArgs = kernelArgs
}

func (d *Driver) HandleLaunchKernelEvent(k *LaunchKernelEvent) error {
	dCoData := d.AllocateMemory(k.Storage, uint64(len(k.CO.Data)))
	d.MemoryCopyHostToDevice(dCoData, k.CO.Data, k.Storage)

	dKernArgData := d.AllocateMemory(k.Storage, uint64(binary.Size(k.KernelArgs)))
	d.MemoryCopyHostToDevice(dKernArgData, k.KernelArgs, k.Storage)

	req := kernels.NewLaunchKernelReq()
	req.HsaCo = k.CO
	req.Packet = new(kernels.HsaKernelDispatchPacket)
	req.Packet.GridSizeX = k.GridSize[0]
	req.Packet.GridSizeY = k.GridSize[1]
	req.Packet.GridSizeZ = k.GridSize[2]
	req.Packet.WorkgroupSizeX = k.WgSize[0]
	req.Packet.WorkgroupSizeY = k.WgSize[1]
	req.Packet.WorkgroupSizeZ = k.WgSize[2]
	req.Packet.KernelObject = uint64(dCoData)
	req.Packet.KernargAddress = uint64(dKernArgData)

	dPacket := d.AllocateMemory(k.Storage, uint64(binary.Size(req.Packet)))
	d.MemoryCopyHostToDevice(dPacket, req.Packet, k.Storage)

	req.PacketAddress = uint64(dPacket)
	req.SetSrc(d)
	req.SetDst(k.GPU)
	req.SetSendTime(0) // FIXME: The time need to be retrieved from the engine
	err := d.GetConnection("ToGPUs").Send(req)
	if err != nil {
		log.Fatal(err)
	}

	return nil
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

func (d *Driver) LaunchKernel(
	co *insts.HsaCo,
	gpu core.Component,
	storage *mem.Storage,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	d.updateLDSPointers(co, kernelArgs)

	dCoData := d.AllocateMemoryWithAlignment(storage, uint64(len(co.Data)), 4096)
	d.MemoryCopyHostToDevice(dCoData, co.Data, storage)

	dKernArgData := d.AllocateMemoryWithAlignment(storage, uint64(binary.Size(kernelArgs)), 4096)
	d.MemoryCopyHostToDevice(dKernArgData, kernelArgs, storage)

	req := kernels.NewLaunchKernelReq()
	req.HsaCo = co
	req.Packet = new(kernels.HsaKernelDispatchPacket)
	req.Packet.GridSizeX = gridSize[0]
	req.Packet.GridSizeY = gridSize[1]
	req.Packet.GridSizeZ = gridSize[2]
	req.Packet.WorkgroupSizeX = wgSize[0]
	req.Packet.WorkgroupSizeY = wgSize[1]
	req.Packet.WorkgroupSizeZ = wgSize[2]
	req.Packet.KernelObject = uint64(dCoData)
	req.Packet.KernargAddress = uint64(dKernArgData)

	dPacket := d.AllocateMemoryWithAlignment(storage, uint64(binary.Size(req.Packet)), 4096)
	d.MemoryCopyHostToDevice(dPacket, req.Packet, storage)

	startTime := d.engine.CurrentTime()
	if startTime < 0 {
		startTime = 0
	}
	req.PacketAddress = uint64(dPacket)
	req.SetSrc(d)
	req.SetDst(gpu)
	req.SetSendTime(startTime) // FIXME: The time need to be retrieved from the engine
	err := d.GetConnection("ToGPUs").Send(req)
	if err != nil {
		log.Fatal(err)
	}

	d.engine.Run()
	endTime := d.engine.CurrentTime()

	fmt.Printf("Kernel: [%.012f - %.012f]\n", startTime, endTime)
}
