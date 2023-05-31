// Package mccl provides a collective communication library implementation.
package mccl

import (
	// For embedded HsaCo files.
	_ "embed"

	"github.com/sarchlab/mgpusim/v3/insts"
	"github.com/sarchlab/mgpusim/v3/kernels"
)

var lastUsedGroupID = 0

//go:embed broadcast.hsaco
var broadcastHsaCoFile []byte

//go:embed allreduce.hsaco
var reduceHsaCoFile []byte

var coPush *insts.HsaCo
var coReduce *insts.HsaCo

func init() {
	coPush = loadKernel(broadcastHsaCoFile, "pushData")
	coReduce = loadKernel(reduceHsaCoFile, "reduceData")
}

func loadKernel(fileContent []byte, kernelName string) *insts.HsaCo {
	co := kernels.LoadProgramFromMemory(fileContent, kernelName)
	if co == nil {
		panic("fail to load pushData kernel")
	}

	return co
}

func computeGPUDist(comms []*Communicator, root int) []int {
	numGPU := len(comms)

	distance := make([]int, numGPU)
	for i := 0; i < numGPU; i++ {
		currID := comms[i].GPUID
		currDist := (currID - root + numGPU) % numGPU
		distance[i] = currDist
	}

	return distance
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
