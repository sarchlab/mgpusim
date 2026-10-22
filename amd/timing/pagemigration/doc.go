// Package pagemigration models runtime page migration between the GPUs of a
// multi-GPU platform.
//
// A Manager plays the role of the driver's migration service. It decides what
// to migrate under one of two policies:
//
//   - on-fault: a vector-memory access of a GPU that translates to a page on
//     another GPU is a fault. The access waits; the fault is serviced in a
//     per-GPU batch after a software latency, and the page (or the group of
//     pages of a larger unit) moves to the faulting GPU.
//   - access counter: every access that leaves the L1 of a GPU for remote
//     memory (a request that the RDMA engine accepts from the L1 side)
//     increments a per-(unit, GPU) counter. When a counter reaches the
//     threshold, the unit migrates, asynchronously, to the GPU with the most
//     accesses. Accesses continue remotely until the shootdown.
//
// Both policies limit thrashing with a cooldown after each migration and a
// cap on the number of migrations per unit, after which the unit is pinned.
//
// A migration of a unit (a page, or an aligned group of pages; the page size
// and the TLBs are not changed) goes through these phases:
//
//		idle -> queued -> service -> shootdown -> drain -> copy -> (remap) -> idle
//
//	 1. queued and service: the unit waits in the software queue of the
//	    destination GPU and is serviced in a batch (SoftwareLatencyNS).
//	 2. shootdown: from here on, every access to the unit is held at its
//	    address translator. The engine of every GPU that accessed the pages
//	    pauses each TLB of the GPU, invalidates the pages, and enables the TLB
//	    again. The phase lasts at least ShootdownLatencyNS.
//	 3. drain: wait until no access already sent to the old frames is in
//	    flight (counted at the address translators).
//	 4. copy: the engine of the destination GPU pulls each page in chunks from
//	    the engine of the source GPU. The source reads through its L2, so
//	    dirty lines are copied; the destination writes the new frame to its
//	    DRAM directly, around the L2.
//	 5. remap: the page table points to the new frames, the old frames are
//	    retired, and the held accesses resume with the new translation.
//
// New frames are taken from a reserved region of each GPU's memory and are
// never reused, so no cache can hold a line of a new frame. Cheap invariant
// checks are always on (no access to a frame being copied or retired, a new
// frame was never accessed); Spec.Check also searches every L2 for lines of
// each new frame.
//
// The observe policy installs the hooks and collects the statistics but
// builds no engine and never migrates; it must give the same timing as no
// page migration at all.
package pagemigration
