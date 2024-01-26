package gpu

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/gpc"
)

type GPUBuilder struct {
	//gpu
	gpuDispatchStrategy string
	gpcCnt              int32

	//gpc
	l2CacheSize int32
	smCntPerGPC int32

	//sm
	l1CacheSize        int32
	smDispatchStrategy string
	smUnitCntPerSM     int32

	//smunit
	l0CacheSize          int32
	registerFileSize     int32
	laneSize             int32
	aluInt32CntPerSMUnit int32
}

func NewGPUBuilder() *GPUBuilder {
	return &GPUBuilder{
		gpuDispatchStrategy:  "",
		l2CacheSize:          0,
		smCntPerGPC:          0,
		l1CacheSize:          0,
		smDispatchStrategy:   "",
		smUnitCntPerSM:       0,
		l0CacheSize:          0,
		registerFileSize:     0,
		laneSize:             0,
		aluInt32CntPerSMUnit: 0,
	}
}

func (g *GPUBuilder) WithGPCCnt(cnt int32) *GPUBuilder {
	g.gpcCnt = cnt
	return g
}

func (g *GPUBuilder) WithSMCnt(cnt int32) *GPUBuilder {
	g.smCntPerGPC = cnt
	return g
}

func (g *GPUBuilder) WithSMUnitCnt(cnt int32) *GPUBuilder {
	g.smUnitCntPerSM = cnt
	return g
}

func (g *GPUBuilder) WithGPUStrategy(strategy string) *GPUBuilder {
	g.gpuDispatchStrategy = strategy
	return g
}

func (g *GPUBuilder) WithSMStrategy(strategy string) *GPUBuilder {
	g.smDispatchStrategy = strategy
	return g
}

func (g *GPUBuilder) WithL2CacheConfig(size int32) *GPUBuilder {
	g.l2CacheSize = size
	return g
}

func (g *GPUBuilder) WithL1CacheConfig(size int32) *GPUBuilder {
	g.l1CacheSize = size
	return g
}

func (g *GPUBuilder) WithL0CacheConfig(size int32) *GPUBuilder {
	g.l0CacheSize = size
	return g
}

func (g *GPUBuilder) WithRegisterFileConfig(registerFileSize int32, laneSize int32) *GPUBuilder {
	g.registerFileSize = registerFileSize
	g.laneSize = laneSize
	return g
}

func (g *GPUBuilder) WithALUConfig(aluType string, num int32) *GPUBuilder {
	switch aluType {
	case "int32":
		g.aluInt32CntPerSMUnit = num
	default:
		panic("ALU type is not supported")
	}
	return g
}

func (g *GPUBuilder) Build() (*GPU, error) {
	gpu := new(GPU)
	gpu.dispatcher = g.buildDispatcher()
	gpu.gpcs = make([]*gpc.GPC, g.gpcCnt)
	for i := range gpu.gpcs {
		gpu.gpcs[i] = gpc.NewGPCBuilder().
			WithSMCnt(g.smCntPerGPC).
			WithSMUnitCnt(g.smUnitCntPerSM).
			WithSMStrategy(g.smDispatchStrategy).
			WithL2CacheSizeConfig(g.l2CacheSize).
			WithL1CacheSizeConfig(g.l1CacheSize).
			WithL0CacheConfig(g.l0CacheSize).
			WithRegisterFileConfig(g.registerFileSize, g.laneSize).
			WithALUConfig("int32", g.aluInt32CntPerSMUnit).
			Build()
	}
	return gpu, nil
}

func (g *GPUBuilder) buildDispatcher() GPUDispatcher {
	switch g.gpuDispatchStrategy {
	case "round-robin":
		return newDispatcherRoundRobin()
	default:
		panic("GPU strategy is not supported")
	}
}
