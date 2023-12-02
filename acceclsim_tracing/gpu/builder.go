package gpu

import (
	"github.com/sarchlab/mgpusim/accelsim_tracing/gpc"
	"github.com/sarchlab/mgpusim/accelsim_tracing/nvidia"
)

type GPU struct {
	meta       *gpuMetaData
	dispatcher gpuDispatcher
	gpcs       []*gpc.GPC
}

type gpuMetaData struct {
	gpcNum    int32
	smNum     int32
	smUnitNum int32

	gpuStrategy    string
	gpcStrategy    string
	smStrategy     string
	smUnitStrategy string

	l2CacheSize int32
	l1CacheSize int32
	l0CacheSize int32

	registerFileSize int32
	laneSize         int32

	alus []struct {
		aluType string
		aluNum  int32
	}
}

func NewGPU() *GPU {
	return &GPU{
		meta: &gpuMetaData{
			gpcNum:    0,
			smNum:     0,
			smUnitNum: 0,

			gpuStrategy:    "default",
			gpcStrategy:    "default",
			smStrategy:     "default",
			smUnitStrategy: "default",

			l2CacheSize: 0,
			l1CacheSize: 0,
			l0CacheSize: 0,

			registerFileSize: 0,
			laneSize:         0,

			alus: nil,
		},
		dispatcher: nil,
		gpcs:       nil,
	}
}

func (g *GPU) WithGPCNum(num int32) *GPU {
	g.meta.gpcNum = num
	return g
}

func (g *GPU) WithSMNum(num int32) *GPU {
	g.meta.smNum = num
	return g
}

func (g *GPU) WithSMUnitNum(num int32) *GPU {
	g.meta.smUnitNum = num
	return g
}

func (g *GPU) WithGPUStrategy(strategy string) *GPU {
	g.meta.gpuStrategy = strategy
	return g
}

func (g *GPU) WithGPCStrategy(strategy string) *GPU {
	g.meta.gpcStrategy = strategy
	return g
}

func (g *GPU) WithSMStrategy(strategy string) *GPU {
	g.meta.smStrategy = strategy
	return g
}

func (g *GPU) WithSMUnitStrategy(strategy string) *GPU {
	g.meta.smUnitStrategy = strategy
	return g
}

func (g *GPU) WithL2CacheSize(size int32) *GPU {
	g.meta.l2CacheSize = size
	return g
}

func (g *GPU) WithL1CacheSize(size int32) *GPU {
	g.meta.l1CacheSize = size
	return g
}

func (g *GPU) WithL0CacheSize(size int32) *GPU {
	g.meta.l0CacheSize = size
	return g
}

func (g *GPU) WithRegisterFileSize(size int32) *GPU {
	g.meta.registerFileSize = size
	return g
}

func (g *GPU) WithLaneSize(size int32) *GPU {
	g.meta.laneSize = size
	return g
}

func (g *GPU) WithALU(aluType string, num int32) *GPU {
	g.meta.alus = append(g.meta.alus, struct {
		aluType string
		aluNum  int32
	}{aluType: aluType, aluNum: num})
	return g
}

func (g *GPU) Build() {
	g.buildDispatcher()
	g.gpcs = make([]*gpc.GPC, g.meta.gpcNum)
	for i := 0; i < int(g.meta.gpcNum); i++ {
		g.gpcs[i] = gpc.NewGPC().WithSMNum(g.meta.smNum).WithSMUnitNum(g.meta.smUnitNum).
			WithGPCStrategy(g.meta.gpcStrategy).WithSMStrategy(g.meta.smStrategy).
			WithSMUnitStrategy(g.meta.smUnitStrategy).WithL2CacheSize(g.meta.l2CacheSize).
			WithL1CacheSize(g.meta.l1CacheSize).WithL0CacheSize(g.meta.l0CacheSize).
			WithRegisterFileSize(g.meta.registerFileSize).WithLaneSize(g.meta.laneSize)
		for _, alu := range g.meta.alus {
			g.gpcs[i].WithALU(alu.aluType, alu.aluNum)
		}
		g.gpcs[i].Build()
	}
}

// [todo] how to handle the relationship between trace.threadblock and truethreadblock
func (g *GPU) RunThreadBlock(tb *nvidia.ThreadBlock) {
	g.dispatcher.dispatch(tb)
}
