package driver

import (
	"encoding/binary"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"reflect"
)

// EnqueueLaunchKernel captures code, dispatch dimensions and argument values.
func (r *Runtime) EnqueueLaunchKernel(
	q CommandQueue,
	co *insts.KernelCodeObject,
	grid [3]uint32,
	group [3]uint16,
	args any,
) CommandID {
	queue := r.queue(q)
	h := queue.Context
	ctx := r.context(h)
	gpu := ctx.GPU
	defer func() { r.context(h).GPU = gpu }()
	gpus := []int{queue.GPU}
	if dev := r.devices[queue.GPU]; dev.Type == internal.DeviceTypeUnifiedGPU {
		gpus = append([]int(nil), dev.UnifiedGPUIDs...)
	}
	cmd := Command{Kind: launchKernel, Code: insts.CodeObjectSnapshot(co), GPUs: gpus}
	for _, device := range gpus {
		ctx.GPU = device
		var code Ptr
		for _, entry := range r.state.Codes {
			if entry.Queue == q && entry.PID == ctx.PID && entry.GPU == device && reflect.DeepEqual(entry.Code, cmd.Code) {
				code = entry.Address
				break
			}
		}
		if code == 0 {
			code = r.AllocateMemory(h, uint64(len(co.Data)))
			r.state.Codes = append(r.state.Codes, codeAddress{q, ctx.PID, device, cmd.Code, code})
			r.EnqueueMemCopyH2D(q, code, co.Data)
		}
		argAddress := r.AllocateMemory(h, co.KernargSegmentByteSize)
		packet := kernels.HsaKernelDispatchPacket{
			GridSizeX:      grid[0],
			GridSizeY:      grid[1],
			GridSizeZ:      grid[2],
			WorkgroupSizeX: group[0],
			WorkgroupSizeY: group[1],
			WorkgroupSizeZ: group[2],
			KernelObject:   uint64(code),
			KernargAddress: uint64(argAddress),
		}
		captured := prepareLocalMemory(co, args, &packet)
		packetAddress := r.AllocateMemory(h, uint64(binary.Size(packet)))
		r.EnqueueMemCopyH2D(q, argAddress, captured)
		r.EnqueueMemCopyH2D(q, packetAddress, packet)
		cmd.Packets = append(cmd.Packets, kernels.PacketSnapshot(&packet))
		cmd.PacketAddresses = append(cmd.PacketAddresses, packetAddress)
	}
	return r.enqueue(q, cmd)
}
func prepareLocalMemory(co *insts.KernelCodeObject, args any, packet *kernels.HsaKernelDispatchPacket) any {
	value := reflect.ValueOf(args)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		panic("runtime: kernel arguments must be a non-nil pointer")
	}
	captured := reflect.New(value.Elem().Type())
	captured.Elem().Set(value.Elem())
	lds := co.GroupSegmentByteSize
	if captured.Elem().Kind() == reflect.Struct {
		fields := captured.Elem()
		for i := 0; i < fields.NumField(); i++ {
			if size, ok := fields.Field(i).Interface().(LocalPtr); ok {
				fields.Field(i).SetUint(uint64(lds))
				lds += uint32(size)
			}
		}
	}
	packet.GroupSegmentSize = lds
	return captured.Interface()
}
