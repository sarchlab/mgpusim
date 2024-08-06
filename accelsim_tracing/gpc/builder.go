package gpc

import (
	"github.com/sarchlab/mgpusim/v4/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v4/accelsim_tracing/sm"
)

type GPC struct {
	meta       *gpcMetaData
	dispatcher gpcDispatcher
	sms        []*sm.SM
}

type gpcMetaData struct {
	smNum     int32
	smUnitNum int32

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

func NewGPC() *GPC {
	return &GPC{
		meta: &gpcMetaData{
			smNum:     0,
			smUnitNum: 0,

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
		sms:        nil,
	}
}

func (g *GPC) WithSMNum(num int32) *GPC {
	g.meta.smNum = num
	return g
}

func (g *GPC) WithGPCStrategy(strategy string) *GPC {
	g.meta.gpcStrategy = strategy
	return g
}

func (g *GPC) WithSMUnitNum(num int32) *GPC {
	g.meta.smUnitNum = num
	return g
}

func (g *GPC) WithSMStrategy(strategy string) *GPC {
	g.meta.smStrategy = strategy
	return g
}

func (g *GPC) WithSMUnitStrategy(strategy string) *GPC {
	g.meta.smUnitStrategy = strategy
	return g
}

func (g *GPC) WithL2CacheSize(size int32) *GPC {
	g.meta.l2CacheSize = size
	return g
}

func (g *GPC) WithL1CacheSize(size int32) *GPC {
	g.meta.l1CacheSize = size
	return g
}

func (g *GPC) WithL0CacheSize(size int32) *GPC {
	g.meta.l0CacheSize = size
	return g
}

func (g *GPC) WithRegisterFileSize(size int32) *GPC {
	g.meta.registerFileSize = size
	return g
}

func (g *GPC) WithLaneSize(size int32) *GPC {
	g.meta.laneSize = size
	return g
}

func (g *GPC) WithALU(aluType string, num int32) *GPC {
	g.meta.alus = append(g.meta.alus, struct {
		aluType string
		aluNum  int32
	}{aluType: aluType, aluNum: num})
	return g
}

func (g *GPC) Build() {
	g.buildDispatcher()
	g.sms = make([]*sm.SM, g.meta.smNum)
	for i := 0; i < int(g.meta.smNum); i++ {
		g.sms[i] = sm.NewSM().
		WithSMStrategy(g.meta.smStrategy).
			WithSMUnitNum(g.meta.smUnitNum).
			WithSMUnitStrategy(g.meta.smUnitStrategy).
			WithL1CacheSize(g.meta.l1CacheSize).
			WithL0CacheSize(g.meta.l0CacheSize).
			WithRegisterFileSize(g.meta.registerFileSize).
			WithLaneSize(g.meta.laneSize)
		for _, alu := range g.meta.alus {
			g.sms[i].WithALU(alu.aluType, alu.aluNum)
		}
		g.sms[i].Build()
	}
}

func (g *GPC) IsFree() bool {
	return true
}

func (g *GPC) Execute(tb *nvidia.ThreadBlock) {
	g.dispatcher.dispatch(tb)
}
