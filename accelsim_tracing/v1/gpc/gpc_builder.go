package gpc

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/sm"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type GPCBuilder struct {
	parentNameString string
	counter          int32

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

func NewGPCBuilder() *GPCBuilder {
	return &GPCBuilder{
		parentNameString: "",
		counter:          0,

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

func (g *GPCBuilder) WithParentNameString(parentNameString string) *GPCBuilder {
	g.parentNameString = parentNameString
	return g
}

func (g *GPCBuilder) WithSMCnt(cnt int32) *GPCBuilder {
	g.smCntPerGPC = cnt
	return g
}

func (g *GPCBuilder) WithSMUnitCnt(cnt int32) *GPCBuilder {
	g.smUnitCntPerSM = cnt
	return g
}

func (g *GPCBuilder) WithSMStrategy(strategy string) *GPCBuilder {
	g.smDispatchStrategy = strategy
	return g
}

func (g *GPCBuilder) WithL2CacheSizeConfig(size int32) *GPCBuilder {
	g.l2CacheSize = size
	return g
}

func (g *GPCBuilder) WithL1CacheSizeConfig(size int32) *GPCBuilder {
	g.l1CacheSize = size
	return g
}

func (g *GPCBuilder) WithL0CacheConfig(size int32) *GPCBuilder {
	g.l0CacheSize = size
	return g
}

func (g *GPCBuilder) WithRegisterFileConfig(registerFileSize int32, laneSize int32) *GPCBuilder {
	g.registerFileSize = registerFileSize
	g.laneSize = laneSize
	return g
}

func (g *GPCBuilder) WithALUConfig(aluType string, cnt int32) *GPCBuilder {
	switch aluType {
	case "int32":
		g.aluInt32CntPerSMUnit = cnt
	default:
		panic("ALU type is not supported")
	}

	return g
}

func (g *GPCBuilder) Build() *GPC {
	gpc := &GPC{
		parentNameString: g.parentNameString,
		nameID:           fmt.Sprintf("%d", g.counter),
	}

	gpc.SMs = make([]runner.TraceableComponent, g.smCntPerGPC)
	smBuilder := sm.NewSMBuilder().
		WithSMUnitCnt(g.smUnitCntPerSM).
		WithSMStrategy(g.smDispatchStrategy).
		WithL1CacheConfig(g.l1CacheSize).
		WithL0CacheConfig(g.l0CacheSize).
		WithRegisterFileConfig(g.registerFileSize, g.laneSize).
		WithALUConfig("int32", g.aluInt32CntPerSMUnit).
		WithParentNameString(gpc.Name())
	for i := range gpc.SMs {
		gpc.SMs[i] = smBuilder.Build()
	}

	g.counter++
	return gpc
}
