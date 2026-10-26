package driver

import "github.com/sarchlab/mgpusim/v5/amd/protocol"

func (m *executionMW) copy(e *Execution) {
	c := m.comp
	r := c.Resources.Runtime
	cmd := &e.Command
	ctx := r.context(r.queue(e.Queue).Context)
	addr := uint64(cmd.Dst)
	size := cmd.ByteSize
	if cmd.Kind == copyD2H {
		addr = uint64(cmd.Src)
	}
	if !c.Spec.UseMagicMemoryCopy {
		for _, buf := range ctx.Buffers {
			if buf.Dirty && memRangeOverlap(uint64(buf.Address), uint64(buf.Address)+buf.Size, addr, addr+uint64(size)) {
				for gpu := range c.Spec.GPUs {
					m.request(e, gpu+1, protocol.FlushReq{}, false)
				}
				break
			}
		}
	}
	offset := uint64(0)
	for offset < uint64(size) {
		page, ok := c.Resources.PageTable.Find(ctx.PID, addr)
		if !ok {
			panic("driver: page not found")
		}
		pageOffset := addr - page.VAddr
		n := min(page.PageSize-pageOffset, uint64(size)-offset)
		paddr := page.PAddr + pageOffset
		if c.Spec.UseMagicMemoryCopy {
			if cmd.Kind == copyH2D {
				c.Resources.GlobalStorage.Write(paddr, r.readBytes(r.queue(e.Queue).Context, cmd.Src+Ptr(offset), n))
			} else {
				r.writeBytes(r.queue(e.Queue).Context, cmd.Dst+Ptr(offset), c.Resources.GlobalStorage.Read(paddr, n))
			}
		} else {
			gpu := r.allocator.GetDeviceIDByPAddr(paddr)
			if cmd.Kind == copyH2D {
				data := r.readBytes(r.queue(e.Queue).Context, cmd.Src+Ptr(offset), n)
				m.request(e, gpu, protocol.MemCopyH2DReq{SrcBuffer: data, DstAddress: paddr}, true)
			} else {
				m.request(e, gpu, protocol.MemCopyD2HReq{SrcAddress: paddr, ByteSize: n, DstOffset: offset}, true)
			}
		}
		addr += n
		offset += n
	}
	if len(e.Requests) == 0 {
		m.finish(len(c.State.Executing) - 1)
		return
	}
	if cmd.Kind == copyH2D {
		c.State.CopyCyclesLeft = c.Spec.H2DCycles
	} else {
		c.State.CopyCyclesLeft = c.Spec.D2HCycles
	}
}
func memRangeOverlap(start1, end1, start2, end2 uint64) bool { return start1 < end2 && start2 < end1 }
