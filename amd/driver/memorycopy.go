package driver

import (
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// copyUnlessMigrating starts a memory copy, unless a unit that it uses is
// migrating: a copy uses physical addresses, so it waits until the unit is
// idle.
func (m *executionMW) copyUnlessMigrating(i int) {
	c := m.comp
	e := &c.State.Executing[i]
	start, size, _ := copyRange(e.Command)

	if m.comp.Middlewares.Migration.migrating(
		c.Resources.Runtime.QueuePID(e.Queue), start, size) {
		e.WaitsForMigration = true
		return
	}

	e.WaitsForMigration = false
	m.copy(i)
}

// startWaitingCopies starts the first memory copy that waited for a
// migration, if it may start now. A copy that finishes at once leaves the
// execution list, so one copy is started per tick.
func (m *executionMW) startWaitingCopies() bool {
	for i := range m.comp.State.Executing {
		if !m.comp.State.Executing[i].WaitsForMigration {
			continue
		}

		m.copyUnlessMigrating(i)

		return i >= len(m.comp.State.Executing) ||
			!m.comp.State.Executing[i].WaitsForMigration
	}

	return false
}

func (m *executionMW) copy(i int) {
	c := m.comp
	e := &c.State.Executing[i]
	r := c.Resources.Runtime
	cmd := &e.Command
	ctx := r.QueueContext(e.Queue)
	addr := uint64(cmd.Dst)
	size := cmd.ByteSize
	if cmd.Kind == gpuruntime.CopyD2H {
		addr = uint64(cmd.Src)
	}
	if !c.Spec.UseMagicMemoryCopy && r.NeedsFlush(ctx, gpuruntime.Ptr(addr), uint64(size)) {
		for gpu := range c.Spec.GPUs {
			m.request(e, gpu+1, protocol.FlushReq{}, false)
		}
	}
	offset := uint64(0)
	for offset < uint64(size) {
		page, ok := c.Resources.PageTable.Find(r.QueuePID(e.Queue), addr)
		if !ok {
			panic("driver: page not found")
		}
		pageOffset := addr - page.VAddr
		n := min(page.PageSize-pageOffset, uint64(size)-offset)
		paddr := page.PAddr + pageOffset
		if c.Spec.UseMagicMemoryCopy {
			if cmd.Kind == gpuruntime.CopyH2D {
				c.Resources.GlobalStorage.Write(paddr, readHostBytes(r, ctx, cmd.Src+gpuruntime.Ptr(offset), n))
			} else {
				writeHostBytes(r, ctx, cmd.Dst+gpuruntime.Ptr(offset), c.Resources.GlobalStorage.Read(paddr, n))
			}
		} else {
			gpu := r.DeviceIDByPAddr(paddr)
			if cmd.Kind == gpuruntime.CopyH2D {
				data := readHostBytes(r, ctx, cmd.Src+gpuruntime.Ptr(offset), n)
				m.request(e, gpu, protocol.MemCopyH2DReq{SrcBuffer: data, DstAddress: paddr}, true)
			} else {
				m.request(e, gpu, protocol.MemCopyD2HReq{SrcAddress: paddr, ByteSize: n, DstOffset: offset}, true)
			}
		}
		addr += n
		offset += n
	}
	if len(e.Requests) == 0 {
		m.finish(i)
		return
	}
	if cmd.Kind == gpuruntime.CopyH2D {
		c.State.CopyCyclesLeft = c.Spec.H2DCycles
	} else {
		c.State.CopyCyclesLeft = c.Spec.D2HCycles
	}
}
func readHostBytes(r *gpuruntime.Runtime, ctx gpuruntime.Context, addr gpuruntime.Ptr, n uint64) []byte {
	data := make([]byte, n)
	if err := r.ReadHost(gpuruntime.HostBuffer{Context: ctx, Address: addr, ByteSize: int(n)}, data); err != nil {
		panic(err)
	}
	return data
}
func writeHostBytes(r *gpuruntime.Runtime, ctx gpuruntime.Context, addr gpuruntime.Ptr, data []byte) {
	if err := r.WriteHost(gpuruntime.HostBuffer{Context: ctx, Address: addr, ByteSize: len(data)}, data); err != nil {
		panic(err)
	}
}
