package driver

import (
	"encoding/binary"
	"log"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/mem"
)

func (d *Driver) LaunchKernel(
	co *insts.HsaCo,
	gpu core.Component,
	storage *mem.Storage,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dCoData := d.AllocateMemory(storage, uint64(len(co.Data)))
	d.MemoryCopyHostToDevice(dCoData, co.Data, storage)

	dKernArgData := d.AllocateMemory(storage, uint64(binary.Size(kernelArgs)))
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

	dPacket := d.AllocateMemory(storage, uint64(binary.Size(req.Packet)))
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

	log.Printf("Kernel: [%.012f - %.012f]\n", startTime, endTime)
}
