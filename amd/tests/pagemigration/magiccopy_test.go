package pagemigration

import "testing"

// The tests in this file show what magic memory copies do on a timing
// platform. A magic copy reads and writes the global storage itself, around
// the GPUs, and flushes nothing: it copies what the global storage has, which
// is not what the caches of a GPU have. The results that the tests expect are
// therefore not the ones of a coherent copy; a simulation that needs those
// uses timed copies.

// A magic download does not see what is only in a cache, and a magic upload
// does not reach a line that a cache has: a dirty line of the L2 is written
// back over it later.
func TestAMagicCopyDoesNotSeeTheDirtyLinesOfTheL2(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, magicCopies)

			const floats = 1024

			written := ramp(1, floats)
			uploaded := ramp(5000, floats)

			// GPU 1 writes a unified buffer with a kernel. The lines are dirty
			// in its L2, and the global storage does not have them.
			staging := s.local(1, floats*floatBytes)
			shared := s.unified(floats * floatBytes)
			s.upload(staging, written)
			s.touch(1, shared, staging, floats)
			s.run()

			if !s.dirtyInL2(s.page(shared).PAddr, floats*floatBytes) {
				t.Fatal("the kernel output is in memory already; the test needs dirty L2 lines")
			}

			// The download returns the zeros of the global storage, not what
			// the kernel wrote.
			mustEqualFloats(t, "the download of what the kernel wrote",
				s.download(shared, floats), make([]float32, floats))

			// The host uploads other data. The global storage has it now, so
			// the host reads it back, while the L2 of GPU 1 keeps the dirty
			// lines of the kernel.
			s.upload(shared, uploaded)
			mustEqualFloats(t, "the download of the upload", s.download(shared, floats), uploaded)

			// GPU 2 reads the buffer, so the page migrates. The migration
			// writes the dirty lines back, over the upload, and copies the
			// page below the L2: the page has what the kernel wrote, and the
			// upload is gone.
			sink := s.local(2, floats*floatBytes)
			s.touch(2, sink, shared, floats)
			s.run()

			if page := s.page(shared); page.DeviceID != 2 {
				t.Fatalf("the page did not migrate: %+v", page)
			}

			mustEqualFloats(t, "the migrated page", s.download(shared, floats), written)
		})
	}
}

// A magic upload does not reach a line that a cache below the L2 has. On
// MI300X the memory below the L2 is the MALL, a cache in front of the DRAM,
// and the global storage is the DRAM: the MALL keeps the lines that it read
// before the upload, and a page copy, which reads the MALL, takes them. On
// R9 Nano the memory below the L2 is the DRAM itself, so the page copy takes
// the upload.
func TestAMagicUploadDoesNotReachTheLinesOfTheMALL(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, magicCopies)

			const floats = 1024

			first := ramp(1, floats)
			second := ramp(5000, floats)

			// GPU 1 reads a unified buffer with a kernel: its caches have the
			// lines now, and none is dirty.
			shared := s.unified(floats * floatBytes)
			s.upload(shared, first)
			s.touch(1, s.local(1, floats*floatBytes), shared, floats)
			s.run()

			// The host uploads other data, into the global storage.
			s.upload(shared, second)

			// GPU 2 reads the buffer, so the page migrates: the L2 of GPU 1
			// drops its lines, and the page is copied from the memory below
			// it.
			s.touch(2, s.local(2, floats*floatBytes), shared, floats)
			s.run()

			if page := s.page(shared); page.DeviceID != 2 {
				t.Fatalf("the page did not migrate: %+v", page)
			}

			want := second
			if gpuType == "mi300x" {
				want = first
			}

			mustEqualFloats(t, "the migrated page", s.download(shared, floats), want)
		})
	}
}
