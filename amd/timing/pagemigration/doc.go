// Package pagemigration models runtime page migration between the GPUs of a
// multi-GPU platform. Every part of it is controlled by messages, and all of
// its state is in the State of the components that run it, so a checkpoint
// saves it and the inspector shows every connection.
//
// # Roles
//
//   - The driver runs the global logic (DriverMiddleware, state in
//     DriverState): it receives the faults and the notifications, services
//     them in batches after a software latency, decides what migrates,
//     allocates the new frames, and updates the page table.
//   - The command processor of each GPU runs the control of that GPU
//     (CPMiddleware, state in CPState): it turns the requests of the driver
//     into control verbs for every address translator and TLB of the GPU
//     and into data-move requests for the data mover of the GPU.
//   - The address translators, the TLBs, and the MMU act only on the
//     messages they receive (Akita's memcontrolprotocol.CmdHold and
//     CmdRelease, TranslationRsp.Fault, vmprotocol.PageFault).
//   - The copy is done by Akita's data mover of the destination GPU: it reads
//     the source page through the RDMA engine of the source GPU (and so
//     through its L2, which holds the dirty lines) and writes the new frame
//     to its own DRAM.
//
// # Triggers
//
//   - on-fault: the vector-memory address translators may fault. A walk of
//     the MMU that finds a page on another GPU, and not pinned, answers at
//     once with a fault and reports it to the driver; the MMU never holds a
//     walk, which avoids deadlocks. The address translator holds the
//     faulted access, and the later accesses to the page, until the driver
//     releases the page, and then translates them again (a replayable
//     fault). A fault is raised only by a walk: an access that hits a TLB
//     entry of a remote page does not fault.
//   - access counter: the RDMA engine of each GPU counts the remote
//     accesses of the GPU to each page and notifies the driver when a count
//     reaches the threshold. The unit migrates, asynchronously, to that GPU.
//
// A unit does not migrate again within the cooldown after a migration, and a
// unit that migrated MaxMigrations times is pinned. A fault on a unit that
// may not migrate maps the unit remotely: its pages are pinned (the MMU does
// not fault on a pinned page), until the end of the cooldown or for good.
//
// # A migration
//
//		idle -> queued -> service -> hold -> copy -> shootdown -> release -> idle
//
//	 1. queued and service: the unit waits in the software queue of the
//	    destination GPU and is serviced in a batch (SoftwareLatencyNS).
//	 2. hold: every GPU holds the pages. A command processor answers once
//	    every address translator of its GPU has no access to the pages and no
//	    translation of them in flight. From here on no GPU accesses the old
//	    frames and no TLB can be filled with the old mapping.
//	 3. copy: the data mover of the destination copies each page to a new
//	    frame.
//	 4. the page table points to the new frames.
//	 5. shootdown, once: every GPU pauses each of its TLBs, invalidates the
//	    pages, and enables the TLB again (at least ShootdownLatencyNS).
//	 6. release: every GPU releases the pages; the held accesses translate
//	    again, miss in the TLBs, and walk to the new mapping.
//
// Because the page table changes while no translation of the pages can be
// in flight, and the shootdown follows the change, no stale translation can
// survive, and no access needs to be corrected.
//
// New frames are taken from a reserved region of each GPU's memory and are
// never reused, so no cache can hold a line of a new frame.
//
// The observe policy only counts the remote accesses; it builds no data
// mover, never migrates, and gives the same timing as no page migration.
package pagemigration
