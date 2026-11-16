package benchmarks

import (
	"encoding/binary"

	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Transfers owns host upload staging until the next globally drained Step.
// Its buffer descriptors are part of the benchmark's actual saved state.
type Transfers struct{ Uploads []gpuruntime.HostBuffer }

func (t *Transfers) Upload(
	r *gpuruntime.Runtime, q gpuruntime.CommandQueue, dst gpuruntime.Ptr, data any,
) gpuruntime.HostBuffer {
	b := Upload(r, q, dst, data)
	t.Uploads = append(t.Uploads, b)
	return b
}

// Upload stages Go data in a CPU buffer and submits its address. The caller
// owns the returned buffer until completion, then releases it or reuses it.
func Upload(r *gpuruntime.Runtime, q gpuruntime.CommandQueue, dst gpuruntime.Ptr, data any) gpuruntime.HostBuffer {
	b := r.AllocateHostBuffer(q, binary.Size(data))
	if err := r.WriteHost(b, data); err != nil {
		panic(err)
	}
	r.EnqueueMemCopyH2D(q, dst, b.Address, b.ByteSize)
	return b
}
func (t *Transfers) ReleaseUploads(r *gpuruntime.Runtime) error {
	for len(t.Uploads) > 0 {
		b := t.Uploads[0]
		if b.Address != 0 {
			if err := r.FreeMemory(b.Context, b.Address); err != nil {
				return err
			}
		}
		t.Uploads = t.Uploads[1:]
	}
	t.Uploads = nil
	return nil
}

// Download allocates a CPU destination and submits an address-to-address copy.
// The caller saves its descriptor, then reads/frees it after a global drain.
func Download(r *gpuruntime.Runtime, q gpuruntime.CommandQueue, src gpuruntime.Ptr, size int) gpuruntime.HostBuffer {
	b := r.AllocateHostBuffer(q, size)
	r.EnqueueMemCopyD2H(q, b.Address, src, size)
	return b
}

// Read decodes and releases a completed destination. Decoding errors leave it
// allocated so the caller can retry. It neither waits nor checks completion.
func Read(r *gpuruntime.Runtime, b gpuruntime.HostBuffer, dst any) error {
	if err := r.ReadHost(b, dst); err != nil {
		return err
	}
	if b.Address == 0 {
		return nil
	}
	return r.FreeMemory(b.Context, b.Address)
}
