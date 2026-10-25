package bfs

import (
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"io"
)

type checkpointState struct {
	Progress         benchmarks.Progress
	InitialConfig    checkpointConfig
	ConfigCaptured   bool
	Depth            int
	FlagResult       driver.CommandID
	OutputResult     driver.CommandID
	Queue            driver.CommandQueue
	Context          driver.Context
	Gpus             []int
	Queues           []driver.CommandQueue
	Arch             arch.Type
	Path             string
	NumNode          int
	Degree           int
	MaxDepth         int
	SourceNode       int
	HEdgeOffsets     []int32
	HEdgeList        []int32
	HFrontier        []uint32
	HCost            []uint32
	CpuCost          []uint32
	DFrontier        driver.Ptr
	DEdgeArray       driver.Ptr
	DEdgeArrayAux    driver.Ptr
	DFlag            driver.Ptr
	UseUnifiedMemory bool
}
type checkpointConfig struct {
	Verification     bool
	Gpus             []int
	Arch             arch.Type
	Path             string
	NumNode          int
	Degree           int
	MaxDepth         int
	UseUnifiedMemory bool
}

func (b *Benchmark) checkpointConfig() checkpointConfig {
	return checkpointConfig{Verification: b.Verification,
		Gpus:             b.gpus,
		Arch:             b.Arch,
		Path:             b.Path,
		NumNode:          b.NumNode,
		Degree:           b.Degree,
		MaxDepth:         b.MaxDepth,
		UseUnifiedMemory: b.useUnifiedMemory,
	}
}
func (b *Benchmark) configuration() checkpointConfig {
	if b.configCaptured {
		return b.initialConfig
	}
	return b.checkpointConfig()
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(w, b.configuration(), checkpointState{
		Progress:         b.Progress,
		InitialConfig:    b.initialConfig,
		ConfigCaptured:   b.configCaptured,
		Depth:            b.depth,
		FlagResult:       b.flagResult,
		OutputResult:     b.outputResult,
		Queue:            b.queue,
		Context:          b.context,
		Gpus:             b.gpus,
		Queues:           b.queues,
		Arch:             b.Arch,
		Path:             b.Path,
		NumNode:          b.NumNode,
		Degree:           b.Degree,
		MaxDepth:         b.MaxDepth,
		SourceNode:       b.sourceNode,
		HEdgeOffsets:     b.hEdgeOffsets,
		HEdgeList:        b.hEdgeList,
		HFrontier:        b.hFrontier,
		HCost:            b.hCost,
		CpuCost:          b.cpuCost,
		DFrontier:        b.dFrontier,
		DEdgeArray:       b.dEdgeArray,
		DEdgeArrayAux:    b.dEdgeArrayAux,
		DFlag:            b.dFlag,
		UseUnifiedMemory: b.useUnifiedMemory,
	}, nil)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s checkpointState
	if err := modeling.ReadCheckpoint(r, b.configuration(), &s, nil); err != nil {
		return err
	}
	b.Progress = s.Progress
	b.initialConfig = s.InitialConfig
	b.configCaptured = s.ConfigCaptured
	b.depth = s.Depth
	b.flagResult = s.FlagResult
	b.outputResult = s.OutputResult
	b.queue = s.Queue
	b.context = s.Context
	b.gpus = s.Gpus
	b.queues = s.Queues
	b.Arch = s.Arch
	b.Path = s.Path
	b.NumNode = s.NumNode
	b.Degree = s.Degree
	b.MaxDepth = s.MaxDepth
	b.sourceNode = s.SourceNode
	b.hEdgeOffsets = s.HEdgeOffsets
	b.hEdgeList = s.HEdgeList
	b.hFrontier = s.HFrontier
	b.hCost = s.HCost
	b.cpuCost = s.CpuCost
	b.dFrontier = s.DFrontier
	b.dEdgeArray = s.DEdgeArray
	b.dEdgeArrayAux = s.DEdgeArrayAux
	b.dFlag = s.DFlag
	b.useUnifiedMemory = s.UseUnifiedMemory
	b.loadProgram()
	return nil
}
