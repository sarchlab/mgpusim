package pagemigration

import (
	"testing"

	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// A page that a kernel has written holds its newest lines only in the L2 of
// its GPU. The migration must write them back before the copy, which reads
// the memory below the L2.
func TestMigrationCopiesTheDirtyLinesOfTheSource(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType)

			const floats = 1024
			data := ramp(1, floats)

			// GPU 1 writes the unified buffer with a kernel.
			staging := s.local(1, floats*floatBytes)
			shared := s.unified(floats * floatBytes)
			s.upload(staging, data)
			s.touch(1, shared, staging, floats)
			s.run()

			// The lines are dirty in the L2 of GPU 1: its memory does not
			// have the data yet.
			frame := s.page(shared)
			if !s.dirtyInL2(frame.PAddr, floats*floatBytes) {
				t.Fatal("the kernel output is in memory already; the test needs dirty L2 lines")
			}

			// GPU 2 reads the buffer: the page faults and migrates.
			result := s.local(2, floats*floatBytes)
			s.touch(2, result, shared, floats)
			s.run()

			moved := s.page(shared)
			if moved.DeviceID != 2 || moved.PAddr == frame.PAddr {
				t.Fatalf("the page did not migrate: %+v", moved)
			}

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			mustEqualFloats(t, "the migrated buffer", s.download(shared, floats), data)

			st := s.stats()
			if st.Faults == 0 || st.Migrations == 0 || st.FramesRetired != st.PagesMigrated {
				t.Fatalf("stats %+v", st)
			}

			t.Logf("%s: %+v", gpuType, st)
		})
	}
}

// With the buddy allocator, the frames of a freed buffer are handed out
// again at once. The L2 of the destination still holds dirty lines of the
// freed buffer; a page that migrates into such a frame must not meet them,
// and they must not be written back over it.
func TestMigrationIntoAFrameThatWasUsedBefore(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, buddyAllocator)

			const floats = 1024

			// GPU 2 writes junk into a page of its own with a kernel, so the
			// lines stay dirty in its L2, and the page is freed.
			junkSource := s.local(2, floats*floatBytes)
			junk := s.local(2, s.pageSize)
			s.upload(junkSource, ramp(-1e6, floats))
			s.touch(2, junk, junkSource, floats)
			s.run()

			freed := s.page(junk).PAddr
			if !s.dirtyInL2(freed, floats*floatBytes) {
				t.Fatal("the junk is in memory already; the test needs dirty L2 lines")
			}

			// A unified page on GPU 1 with data in the same place. GPU 2
			// reads it with the next kernel, so the page migrates before any
			// cache is flushed. The junk page is freed after everything else
			// that GPU 2 needs is allocated, so its frame goes to the page
			// that migrates.
			shared := s.unified(s.pageSize)
			result := s.local(2, floats*floatBytes)
			data := ramp(1, floats)

			s.upload(shared, data)
			s.touch(2, result, shared, floats)
			s.padBuddy(2, 2)
			s.free(junk)
			s.run()

			if page := s.page(shared); page.DeviceID != 2 || page.PAddr != freed {
				t.Fatalf("the page is in frame %#x of GPU %d, want the freed frame %#x of GPU 2",
					page.PAddr, page.DeviceID, freed)
			}

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			mustEqualFloats(t, "the migrated page", s.download(shared, floats), data)
			t.Logf("%s: %+v", gpuType, s.stats())
		})
	}
}

// writeJunk lets a GPU write junk into a page of its own with a kernel, so
// that the lines stay dirty in its L2, and returns the page and its frame.
// Nothing flushes the L2 until the page is freed and its frame is used again.
func (s *system) writeJunk(gpu, floats int) (junk gpuruntime.Ptr, frame uint64) {
	s.t.Helper()

	source := s.local(gpu, uint64(floats)*floatBytes)
	junk = s.local(gpu, s.pageSize)
	s.upload(source, ramp(-1e6, floats))
	s.touch(gpu, junk, source, floats)
	s.run()

	frame = s.page(junk).PAddr
	if !s.dirtyInL2(frame, uint64(floats)*floatBytes) {
		s.t.Fatal("the junk is in memory already; the test needs dirty L2 lines")
	}

	return junk, frame
}

// An upload into a page that was used before, whose frame can still have
// dirty lines in the L2 of its GPU, must not be hidden by them, and the
// migration of the page must not write them back over the upload: the caches
// are flushed before the upload.
func TestAnUploadIntoAFrameThatWasUsedBefore(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, buddyAllocator)

			const floats = 1024

			// A unified page gets the frame of a junk page of GPU 1, and the
			// host uploads data into it.
			junk, freed := s.writeJunk(1, floats)
			result := s.local(2, floats*floatBytes)
			s.padBuddy(1, 2)
			s.free(junk)

			shared := s.unified(s.pageSize)
			if frame := s.page(shared).PAddr; frame != freed {
				t.Fatalf("the page is in frame %#x, want the freed frame %#x", frame, freed)
			}

			data := ramp(1, floats)
			s.upload(shared, data)

			// GPU 2 reads the page, which migrates out of that frame.
			s.touch(2, result, shared, floats)
			s.run()

			if page := s.page(shared); page.DeviceID != 2 {
				t.Fatalf("the page did not migrate: %+v", page)
			}

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			mustEqualFloats(t, "the migrated page", s.download(shared, floats), data)
			t.Logf("%s: %+v", gpuType, s.stats())
		})
	}
}

// Distribute moves the pages of a buffer to several GPUs. A page that it
// puts into a frame that was used before must not meet the dirty L2 lines
// that the last user of the frame left: the caches are flushed before the
// host copies into the buffer.
func TestAnUploadIntoADistributedFrameThatWasUsedBefore(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, buddyAllocator)

			const floats = 1024

			// The second page of a new buffer of GPU 1 is distributed to
			// GPU 2, into the frame of a junk page.
			junk, freed := s.writeJunk(2, floats)
			result := s.local(2, floats*floatBytes)
			buffer := s.local(1, 2*s.pageSize)
			s.padBuddy(2, 2)
			s.free(junk)
			s.rt.Distribute(s.ctx, buffer, 2*s.pageSize, []int{1, 2})

			second := s.at(buffer, 1, 0)
			if page := s.page(second); page.DeviceID != 2 || page.PAddr != freed {
				t.Fatalf("the page is in frame %#x of GPU %d, want the freed frame %#x of GPU 2",
					page.PAddr, page.DeviceID, freed)
			}

			// The host uploads data into the page, and GPU 2 reads it.
			data := ramp(1, floats)
			s.upload(second, data)
			s.touch(2, result, second, floats)
			s.run()

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			mustEqualFloats(t, "the distributed page", s.download(second, floats), data)
		})
	}
}

// remapDestinations are the devices that a page is remapped to in order to
// get a frame of GPU 2: GPU 2 itself, and a unified GPU, which has no memory
// of its own and takes its first frames from GPU 2.
var remapDestinations = []struct {
	name   string
	device func(s *system) int
}{
	{"to-the-gpu", func(*system) int { return 2 }},
	{"to-a-unified-gpu", func(s *system) int { return s.rt.CreateUnifiedGPU(s.ctx, []int{2, 1}) }},
}

// Contexts made with InitWithExistingPID address the same memory, so what the
// caches hold of a buffer concerns the copies of all of them. Here a page of
// a buffer of the first context is remapped into a frame with dirty L2
// lines, and a second context uploads into the page: the upload must flush
// the lines, although the second context has no buffer there. The page is on
// GPU 2 also when it is remapped to a unified GPU.
func TestAnotherContextUploadsIntoARemappedFrameThatWasUsedBefore(t *testing.T) {
	for _, gpuType := range platforms {
		for _, destination := range remapDestinations {
			t.Run(gpuType+"/"+destination.name, func(t *testing.T) {
				s := newSystem(t, gpuType, buddyAllocator)
				other := s.sibling()

				const floats = 1024

				junk, freed := s.writeJunk(2, floats)
				result := s.local(2, floats*floatBytes)
				buffer := s.local(1, 2*s.pageSize)
				second := s.at(buffer, 1, 0)

				s.padBuddy(2, 2)
				s.free(junk)
				s.rt.Remap(s.ctx, uint64(second), s.pageSize, destination.device(s))

				if page := s.page(second); page.DeviceID != 2 || page.PAddr != freed {
					t.Fatalf("the page is in frame %#x of device %d, want the freed frame %#x of GPU 2",
						page.PAddr, page.DeviceID, freed)
				}

				// The other context uploads. A kernel of the first context
				// reads the page on GPU 2.
				data := ramp(1, floats)
				other.upload(second, data)
				s.touch(2, result, second, floats)
				s.run()

				mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
				mustEqualFloats(t, "the remapped page", other.download(second, floats), data)
			})
		}
	}
}

// A kernel can write the buffers of every context of its address space. What
// a kernel of a second context wrote into a buffer of the first is still in
// the caches: a download must flush them first, whether it comes through the
// context that ran the kernel or through the one that has the buffer.
func TestADownloadOfWhatTheKernelOfAnotherContextWrote(t *testing.T) {
	for _, gpuType := range platforms {
		for _, through := range []string{"kernel-context", "buffer-context"} {
			t.Run(gpuType+"/"+through, func(t *testing.T) {
				s := newSystem(t, gpuType)
				other := s.sibling()

				const floats = 1024

				// No kernel has run yet, so the buffers are clean.
				data := ramp(1, floats)
				source := s.local(1, floats*floatBytes)
				output := s.local(1, floats*floatBytes)
				s.upload(source, data)

				other.touch(1, output, source, floats)
				other.run()

				if !s.dirtyInL2(s.page(output).PAddr, floats*floatBytes) {
					t.Fatal("the kernel output is in memory already; the test needs dirty L2 lines")
				}

				reader := s
				if through == "kernel-context" {
					reader = other
				}

				mustEqualFloats(t, "the download", reader.download(output, floats), data)
			})
		}
	}
}
