package mccl

import (
	"gitlab.com/akita/mgpusim/v2/driver"
)

// Communicator is a struct that helps gpu to communicate.
type Communicator struct {
	Driver    *driver.Driver
	Ctx       *driver.Context
	GPUID     int
	Rank      uint32
	GroupID   int
	GroupSize uint32
}

// NewCommunicator creates a new communicator
// rank is the id of this gpu
func NewCommunicator(
	driver *driver.Driver,
	ctx *driver.Context,
	gpuID int,
	rank uint32,
	groupID int,
	groupSize uint32,
) *Communicator {
	comm := &Communicator{
		driver,
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
	driver *driver.Driver,
	ctx *driver.Context,
	gpuIDs []int,
) []*Communicator {
	lastUsedGroupID++
	var comms []*Communicator
	for i := 0; i < nDev; i++ {
		newComm := NewCommunicator(
			driver,
			ctx,
			gpuIDs[i],
			uint32(i),
			lastUsedGroupID,
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
	driver *driver.Driver,
	ctxs []*driver.Context,
	gpuIDs []int,
) []*Communicator {
	lastUsedGroupID++
	var comms []*Communicator
	for i := 0; i < nDev; i++ {
		newComm := NewCommunicator(
			driver,
			ctxs[i],
			gpuIDs[i],
			uint32(i),
			lastUsedGroupID,
			uint32(nDev),
		)
		comms = append(comms, newComm)
	}
	return comms
}
