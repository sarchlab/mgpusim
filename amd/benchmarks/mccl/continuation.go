package mccl

import (
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// Collective contains only saved handles and phase indices. Its caller owns
// the checkpoint and provides the Runtime on each post-drain Step.
type Collective struct {
	Kind                   string
	Comms                  []Communicator
	Queues                 []driver.CommandQueue
	Data, Buffers          []driver.Ptr
	Size, BufferSize, Root int
	Chunk, Stage           int
	Ended                  bool
}

func copyCommunicators(comms []*Communicator) []Communicator {
	out := make([]Communicator, len(comms))
	for i, c := range comms {
		out[i] = *c
	}
	return out
}
func (c *Collective) communicators() []*Communicator {
	out := make([]*Communicator, len(c.Comms))
	for i := range c.Comms {
		out[i] = &c.Comms[i]
	}
	return out
}
func (c *Collective) IsEnded() bool { return c.Ended }
func (c *Collective) Step(d *driver.Runtime) error {
	if c.Ended {
		return nil
	}
	for _, q := range c.Queues {
		if counts := d.Unfinished(); int(q) > len(counts) || counts[q-1] != 0 {
			return fmt.Errorf("collective step before queue %d drained", q)
		}
	}
	if len(c.Comms) == 0 {
		return fmt.Errorf("collective requires participants")
	}
	if len(c.Comms) == 1 || c.Size == 0 {
		c.Ended = true
		return nil
	}
	if c.Kind == "broadcast" {
		c.broadcast(d)
		return nil
	}
	if c.Kind != "reduce" || c.BufferSize <= 0 {
		return fmt.Errorf("invalid collective configuration")
	}
	if c.Chunk*c.BufferSize >= c.Size {
		c.Ended = true
		return nil
	}
	n := len(c.Comms)
	size := min(c.BufferSize, c.Size-c.Chunk*c.BufferSize)
	each := (size + n - 1) / n
	comms := c.communicators()
	offset := uint64(c.Chunk * c.BufferSize)
	if c.Stage < 2*(n-1) {
		if c.Stage%2 == 0 {
			allReducePushToBuffer(
				d,
				comms,
				c.Queues,
				c.Data,
				c.Buffers,
				c.Stage/2,
				n,
				1024,
				uint64(size),
				uint64(each),
				offset,
			)
		} else {
			allReduceReduce(d, comms, c.Queues, c.Data, c.Buffers, c.Stage/2, n, 1024, uint64(size), uint64(each), offset)
		}
	} else {
		allReducePushToGPU(d, comms, c.Queues, c.Data, c.Stage-2*(n-1), n, 1024, uint64(size), uint64(each), offset)
	}
	c.Stage++
	if c.Stage == 3*(n-1) {
		c.Stage = 0
		c.Chunk++
	}
	return nil
}
