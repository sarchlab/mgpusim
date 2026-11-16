package pagemigration

import (
	"testing"

	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// contention are the platforms on which several GPUs fight for pages with
// the default specification: every GPU of the platform works on the same
// unified pages at the same time.
var contention = []struct {
	gpuType string
	gpus    int
}{
	{"r9nano", 4},
	{"mi300x", 2},
}

// Every GPU reads the same unified buffer at the same time. Whatever the
// pages do, migrate, wait for a throttled GPU, or stay pinned and be read
// remotely, every GPU must read the data.
func TestAllGPUsReadTheSamePages(t *testing.T) {
	for _, c := range contention {
		t.Run(c.gpuType, func(t *testing.T) {
			s := newSystem(t, c.gpuType, gpus(c.gpus))

			const floats = 4 * 1024

			data := ramp(1, floats)
			shared := s.unified(floats * floatBytes)
			s.upload(shared, data)

			results := make([]gpuruntime.Ptr, c.gpus)
			for gpu := 1; gpu <= c.gpus; gpu++ {
				results[gpu-1] = s.local(gpu, floats*floatBytes)
				s.touch(gpu, results[gpu-1], shared, floats)
			}

			s.run()

			for gpu := 1; gpu <= c.gpus; gpu++ {
				mustEqualFloats(t, "what a GPU read", s.download(results[gpu-1], floats), data)
			}

			st := s.stats()
			if st.Migrations == 0 {
				t.Fatalf("no page migrated: %+v", st)
			}

			t.Logf("%s, %d GPUs: %+v", c.gpuType, c.gpus, st)
		})
	}
}

// Every GPU writes its own part of the same unified page at the same time,
// again and again. The page moves between the GPUs with its dirty lines, or
// is written remotely while it is pinned; in the end it must hold what every
// GPU wrote last.
func TestAllGPUsWriteTheSamePage(t *testing.T) {
	for _, c := range contention {
		t.Run(c.gpuType, func(t *testing.T) {
			s := newSystem(t, c.gpuType, gpus(c.gpus))

			const (
				part   = 4 * groupFloats // what one GPU writes
				rounds = 3
			)

			shared := s.unified(uint64(c.gpus) * part * floatBytes)
			sources := make([]gpuruntime.Ptr, c.gpus)

			for gpu := 1; gpu <= c.gpus; gpu++ {
				sources[gpu-1] = s.local(gpu, part*floatBytes)
			}

			var want []float32

			for round := range rounds {
				want = want[:0]

				for gpu := 1; gpu <= c.gpus; gpu++ {
					data := ramp(float32(1000*(round+1)+100*gpu), part)
					want = append(want, data...)
					s.upload(sources[gpu-1], data)
				}

				for gpu := 1; gpu <= c.gpus; gpu++ {
					s.touch(gpu, shared+gpuruntime.Ptr((gpu-1)*part*floatBytes), sources[gpu-1], part)
				}

				s.run()
			}

			mustEqualFloats(t, "the shared page", s.download(shared, c.gpus*part), want)

			st := s.stats()
			if st.Migrations == 0 {
				t.Fatalf("no page migrated: %+v", st)
			}

			t.Logf("%s, %d GPUs: %+v", c.gpuType, c.gpus, st)
		})
	}
}
