package runtime

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// HostBuffer describes CPU storage in a Runtime address space. It contains no
// Go memory pointer. Its owner controls lifetime and synchronization.
type HostBuffer struct {
	Context  Context
	Address  Ptr
	ByteSize int
}

// AllocateHostBuffer allocates CPU pages in the queue's address space without
// changing its context's selected GPU.
func (r *Runtime) AllocateHostBuffer(q CommandQueue, size int) HostBuffer {
	if size < 0 {
		panic("runtime: host buffer cannot have negative size")
	}
	h := r.queue(q).Context
	if size == 0 {
		return HostBuffer{Context: h}
	}
	ctx := r.context(h)
	ptr := Ptr(r.allocator.Allocate(ctx.PID, uint64(size), 0))
	ctx.Buffers = append(ctx.Buffers, buffer{Address: ptr, Size: uint64(size)})
	return HostBuffer{Context: h, Address: ptr, ByteSize: size}
}

// WriteHost encodes Go values into CPU storage without advancing time. The
// caller must not overwrite a source still used by submitted work.
func (r *Runtime) WriteHost(b HostBuffer, src any) error {
	var data bytes.Buffer
	if err := binary.Write(&data, binary.LittleEndian, src); err != nil {
		return err
	}
	if data.Len() != b.ByteSize {
		return fmt.Errorf("runtime: host buffer size mismatch")
	}
	r.writeBytes(b.Context, b.Address, data.Bytes())
	return nil
}

// ReadHost decodes CPU storage without advancing time. It does not wait for a
// submitted copy, consume the buffer, or check whether work still uses it.
func (r *Runtime) ReadHost(b HostBuffer, dst any) error {
	if binary.Size(dst) != b.ByteSize {
		return fmt.Errorf("runtime: host buffer size mismatch")
	}
	return binary.Read(bytes.NewReader(r.readBytes(b.Context, b.Address, uint64(b.ByteSize))), binary.LittleEndian, dst)
}
func (r *Runtime) readBytes(h Context, addr Ptr, size uint64) []byte {
	out := make([]byte, size)
	r.hostPages(h, addr, size, func(offset, paddr, n uint64) { copy(out[offset:offset+n], r.storage.Read(paddr, n)) })
	return out
}
func (r *Runtime) writeBytes(h Context, addr Ptr, data []byte) {
	r.hostPages(h, addr, uint64(len(data)), func(offset, paddr, n uint64) {
		r.storage.Write(paddr, data[offset:offset+n])
	})
}
func (r *Runtime) hostPages(h Context, addr Ptr, size uint64, visit func(uint64, uint64, uint64)) {
	pid := r.context(h).PID
	for offset := uint64(0); offset < size; {
		page, ok := r.pageTable.Find(pid, uint64(addr)+offset)
		if !ok {
			panic("runtime: host page not found")
		}
		inPage := uint64(addr) + offset - page.VAddr
		n := min(page.PageSize-inPage, size-offset)
		visit(offset, page.PAddr+inPage, n)
		offset += n
	}
}

// stageUpload is only for Runtime-owned code/dispatch snapshots. Ordinary H2D
// submissions take addresses and never snapshot caller data.
func (r *Runtime) stageUpload(q CommandQueue, dst Ptr, data any) {
	b := r.AllocateHostBuffer(q, binary.Size(data))
	if err := r.WriteHost(b, data); err != nil {
		panic(err)
	}
	cmd := Command{Kind: CopyH2D, Src: b.Address, Dst: dst, ByteSize: b.ByteSize}
	if b.Address != 0 {
		cmd.Owned = []Ptr{b.Address}
	}
	r.enqueue(q, cmd)
}
