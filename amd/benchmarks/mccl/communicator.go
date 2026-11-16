package mccl

import (
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Communicator is a struct that helps gpu to communicate.
type Communicator struct {
	Ctx       gpuruntime.Context
	GPUID     int
	Rank      uint32
	GroupID   int
	GroupSize uint32
}

// NewCommunicator creates a new communicator
// rank is the id of this gpu
func NewCommunicator(
	rt *gpuruntime.Runtime,
	ctx gpuruntime.Context,
	gpuID int,
	rank uint32,
	groupID int,
	groupSize uint32,
) *Communicator {
	comm := &Communicator{
		ctx,
		gpuID,
		rank,
		groupID,
		groupSize,
	}
	return comm
}

// CommInitAll initiates a list of communicators.
func CommInitAll(
	nDev int,
	rt *gpuruntime.Runtime,
	ctx gpuruntime.Context,
	gpuIDs []int,
) []*Communicator {
	var comms []*Communicator
	for i := 0; i < nDev; i++ {
		newComm := NewCommunicator(
			rt,
			ctx,
			gpuIDs[i],
			uint32(i),
			0,
			uint32(nDev),
		)
		comms = append(comms, newComm)
	}

	return comms
}

// CommInitAllMultipleContexts creates a list of communicators with one
// context per communicator.
func CommInitAllMultipleContexts(
	nDev int,
	rt *gpuruntime.Runtime,
	ctxs []gpuruntime.Context,
	gpuIDs []int,
) []*Communicator {
	var comms []*Communicator
	for i := 0; i < nDev; i++ {
		newComm := NewCommunicator(
			rt,
			ctxs[i],
			gpuIDs[i],
			uint32(i),
			0,
			uint32(nDev),
		)
		comms = append(comms, newComm)
	}
	return comms
}
