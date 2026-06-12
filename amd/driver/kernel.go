package driver

import (
	"encoding/binary"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v4/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

// EnqueueLaunchKernel schedules kernel to be launched later
func (d *Driver) EnqueueLaunchKernel(
	queue *CommandQueue,
	co *insts.KernelCodeObject,
	gridSize [3]uint32,
	wgSize [3]uint16,
	kernelArgs interface{},
) {
	dev := d.devices[queue.GPUID]

	if dev.Type == internal.DeviceTypeUnifiedGPU {
		d.enqueueLaunchUnifiedKernel(queue, co, gridSize, wgSize, kernelArgs)
	} else {
		dCoData, cached := d.codeObjGPUAddrs[co]
		if !cached {
			if len(co.Relocations) > 0 {
				dCoData = d.allocateAndPatchRelocatedCode(
					queue, co)
			} else {
				dCoData = d.AllocateMemory(queue.Context,
					uint64(len(co.Data)))
				d.EnqueueMemCopyH2D(queue, dCoData, co.Data)
			}
			d.codeObjGPUAddrs[co] = dCoData
		}

		dKernArgData := d.AllocateMemory(queue.Context, co.KernargSegmentByteSize)

		packet := kernels.HsaKernelDispatchPacket{}
		dPacket := d.AllocateMemory(queue.Context, uint64(binary.Size(packet)))

		aqlPacket := d.createAQLPacket(gridSize, wgSize, dCoData, dKernArgData)
		newKernelArgs := d.prepareLocalMemory(co, kernelArgs, aqlPacket)

		d.EnqueueMemCopyH2D(queue, dKernArgData, newKernelArgs)
		d.fillV5ImplicitKernArgs(queue, co, dKernArgData, aqlPacket)
		d.EnqueueMemCopyH2D(queue, dPacket, aqlPacket)

		d.enqueueLaunchKernelCommand(queue, co, aqlPacket, dPacket)
	}
}

func (d *Driver) allocateGPUMemory(
	ctx *Context,
	co *insts.KernelCodeObject,
) (dCoData, dKernArgData, dPacket Ptr) {
	dCoData = d.AllocateMemory(ctx, uint64(len(co.Data)))
	dKernArgData = d.AllocateMemory(ctx, co.KernargSegmentByteSize)

	packet := kernels.HsaKernelDispatchPacket{}
	dPacket = d.AllocateMemory(ctx, uint64(binary.Size(packet)))

	return dCoData, dKernArgData, dPacket
}

func (d *Driver) prepareLocalMemory(
	co *insts.KernelCodeObject,
	kernelArgs interface{},
	packet *kernels.HsaKernelDispatchPacket,
) (newKernelArgs interface{}) {
	newKernelArgs = reflect.New(reflect.TypeOf(kernelArgs).Elem()).Interface()
	reflect.ValueOf(newKernelArgs).Elem().
		Set(reflect.ValueOf(kernelArgs).Elem())

	ldsSize := co.GroupSegmentByteSize

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

// LaunchKernel is an easy way to run a kernel on the GPU simulator. It
// launches the kernel immediately.
func (d *Driver) LaunchKernel(
	ctx *Context,
	co *insts.KernelCodeObject,
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
	co *insts.KernelCodeObject,
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

// v5ImplicitKernArgSize is the size of the implicit kernarg region for
// AMDHSA Code Object V5 (and V4). This is always 256 bytes per the LLVM
// AMDHSA specification.
const v5ImplicitKernArgSize = 256

// fillV5ImplicitKernArgs writes the implicit (hidden) kernel arguments
// that V5 kernels expect at the end of the kernarg segment. These include
// block counts, group sizes, remainders, global offsets, and grid dims.
//
// ROCm V5 kernels (compiled with ROCm clang) read hipBlockDim_x,
// hipGridDim_x, etc. from these implicit args rather than from the
// dispatch pointer. Without filling these, multi-workgroup dispatches
// produce incorrect results.
//
// Layout (offsets relative to implicit arg start, per AMDHSA V5 spec):
//
//	+0:  block_count_x    (uint32)
//	+4:  block_count_y    (uint32)
//	+8:  block_count_z    (uint32)
//	+12: group_size_x     (uint16)
//	+14: group_size_y     (uint16)
//	+16: group_size_z     (uint16)
//	+18: remainder_x      (uint16)
//	+20: remainder_y      (uint16)
//	+22: remainder_z      (uint16)
//	+24: reserved         (16 bytes)
//	+40: global_offset_x  (uint64)
//	+48: global_offset_y  (uint64)
//	+56: global_offset_z  (uint64)
//	+64: grid_dims        (uint16)
func (d *Driver) fillV5ImplicitKernArgs(
	queue *CommandQueue,
	co *insts.KernelCodeObject,
	dKernArgData Ptr,
	packet *kernels.HsaKernelDispatchPacket,
) {
	if co.Version != insts.CodeObjectV5 {
		return
	}

	if co.KernargSegmentByteSize < v5ImplicitKernArgSize {
		return
	}

	implicitOffset := co.KernargSegmentByteSize - v5ImplicitKernArgSize

	// Build the 256-byte implicit arg buffer
	buf := make([]byte, v5ImplicitKernArgSize)

	// Block counts = ceil(gridSize / groupSize)
	blockCountX := ceilDiv(packet.GridSizeX, uint32(packet.WorkgroupSizeX))
	blockCountY := ceilDiv(packet.GridSizeY, uint32(packet.WorkgroupSizeY))
	blockCountZ := ceilDiv(packet.GridSizeZ, uint32(packet.WorkgroupSizeZ))

	binary.LittleEndian.PutUint32(buf[0:4], blockCountX)
	binary.LittleEndian.PutUint32(buf[4:8], blockCountY)
	binary.LittleEndian.PutUint32(buf[8:12], blockCountZ)

	// Group sizes
	binary.LittleEndian.PutUint16(buf[12:14], packet.WorkgroupSizeX)
	binary.LittleEndian.PutUint16(buf[14:16], packet.WorkgroupSizeY)
	binary.LittleEndian.PutUint16(buf[16:18], packet.WorkgroupSizeZ)

	// Remainders = gridSize % groupSize
	binary.LittleEndian.PutUint16(buf[18:20],
		uint16(packet.GridSizeX%uint32(packet.WorkgroupSizeX)))
	binary.LittleEndian.PutUint16(buf[20:22],
		uint16(packet.GridSizeY%uint32(packet.WorkgroupSizeY)))
	binary.LittleEndian.PutUint16(buf[22:24],
		uint16(packet.GridSizeZ%uint32(packet.WorkgroupSizeZ)))

	// Reserved [24:40] = 0 (16 bytes, already zeroed)

	// Global offsets [40:64] = 0 (already zeroed for standard dispatches)

	// Grid dims: count of non-trivial dimensions
	gridDims := uint16(1)
	if packet.GridSizeY > 1 || packet.WorkgroupSizeY > 1 {
		gridDims = 2
	}
	if packet.GridSizeZ > 1 || packet.WorkgroupSizeZ > 1 {
		gridDims = 3
	}
	binary.LittleEndian.PutUint16(buf[64:66], gridDims)

	// Write implicit args at the correct offset in kernarg memory
	d.EnqueueMemCopyH2D(queue, dKernArgData+Ptr(implicitOffset), buf)
}

func ceilDiv(a, b uint32) uint32 {
	if b == 0 {
		return 0
	}
	return (a + b - 1) / b
}

func (d *Driver) enqueueLaunchUnifiedKernelCommand(
	queue *CommandQueue,
	co *insts.KernelCodeObject,
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
	co *insts.KernelCodeObject,
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
		d.fillV5ImplicitKernArgs(queue, co, dKernArgData, packet)
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

// allocateAndPatchRelocatedCode resolves R_AMDGPU_GOTPCREL32_LO/HI
// relocations. It allocates GPU memory for the kernel's global data
// sections, builds a GOT with entries pointing to those sections,
// patches the code's literal constants with PC-relative offsets to the
// GOT entries, and uploads everything.
//
// Returns the GPU address of the patched code.
//
//nolint:funlen,gocyclo
func (d *Driver) allocateAndPatchRelocatedCode(
	queue *CommandQueue,
	co *insts.KernelCodeObject,
) Ptr {
	const (
		gotpcrelLo = 8 // R_AMDGPU_GOTPCREL32_LO
		gotpcrelHi = 9 // R_AMDGPU_GOTPCREL32_HI
	)

	ctx := queue.Context

	// Assign GOT slot indices to unique symbols
	symGOTIndex := make(map[string]int)
	type gotEntry struct {
		symValue uint64
		symSec   string
	}
	var gotEntries []gotEntry

	for _, r := range co.Relocations {
		if r.Type != gotpcrelLo && r.Type != gotpcrelHi {
			continue
		}
		if _, exists := symGOTIndex[r.SymName]; !exists {
			symGOTIndex[r.SymName] = len(gotEntries)
			gotEntries = append(gotEntries, gotEntry{
				symValue: r.SymValue,
				symSec:   r.SymSec,
			})
		}
	}

	// Allocate GPU memory for each data section
	sectionGPU := make(map[string]Ptr)
	for _, ds := range co.DataSections {
		addr := d.AllocateMemory(ctx, uint64(len(ds.Data)))
		sectionGPU[ds.Name] = addr
		d.EnqueueMemCopyH2D(queue, addr, ds.Data)
	}

	// Build GOT buffer (8 bytes per entry = absolute symbol address)
	gotSize := uint64(len(gotEntries) * 8)
	gotBuf := make([]byte, gotSize)

	for i, ge := range gotEntries {
		secBase, ok := sectionGPU[ge.symSec]
		if !ok {
			log.Printf("reloc: symbol section %q not loaded", ge.symSec)
			continue
		}
		symAddr := uint64(secBase) + ge.symValue
		binary.LittleEndian.PutUint64(gotBuf[i*8:], symAddr)
	}

	var gotGPU Ptr
	if gotSize > 0 {
		gotGPU = d.AllocateMemory(ctx, gotSize)
		d.EnqueueMemCopyH2D(queue, gotGPU, gotBuf)
	}

	// Allocate code memory so we know the base address for patching
	codeGPU := d.AllocateMemory(ctx, uint64(len(co.Data)))

	// Patch the code's literal constants
	patched := make([]byte, len(co.Data))
	copy(patched, co.Data)

	for _, r := range co.Relocations {
		idx, ok := symGOTIndex[r.SymName]
		if !ok {
			continue
		}
		if r.Offset+4 > uint64(len(patched)) {
			continue
		}

		gotAddr := int64(gotGPU) + int64(idx*8)
		place := int64(codeGPU) + int64(r.Offset)
		val := gotAddr + r.Addend - place

		switch r.Type {
		case gotpcrelLo:
			binary.LittleEndian.PutUint32(
				patched[r.Offset:], uint32(val))
		case gotpcrelHi:
			binary.LittleEndian.PutUint32(
				patched[r.Offset:], uint32(val>>32))
		}
	}

	d.EnqueueMemCopyH2D(queue, codeGPU, patched)

	return codeGPU
}
