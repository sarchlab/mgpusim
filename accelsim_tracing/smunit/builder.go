package smunit

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/alu"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type SMUnit struct {
	meta         *smUnitMetaData
	dispatcher   smUnitDispatcher
	registerFile *RegisterFile
	aluGroup     []*alu.ALUGroup
}

type smUnitMetaData struct {
	smUnitStrategy string

	l0CacheSize int32

	registerFileSize int32
	laneSize         int32

	alus []struct {
		aluType string
		aluNum  int32
	}
}

func NewSMUnit() *SMUnit {
	return &SMUnit{
		meta: &smUnitMetaData{
			smUnitStrategy: "default",

			l0CacheSize: 0,

			registerFileSize: 0,
			laneSize:         0,

			alus: nil,
		},
		dispatcher:   nil,
		registerFile: nil,
		aluGroup:     nil,
	}
}

func (s *SMUnit) WithSMUnitStrategy(strategy string) *SMUnit {
	s.meta.smUnitStrategy = strategy
	return s
}

func (s *SMUnit) WithL0CacheSize(size int32) *SMUnit {
	s.meta.l0CacheSize = size
	return s
}

func (s *SMUnit) WithRegisterFileSize(size int32) *SMUnit {
	s.meta.registerFileSize = size
	return s
}

func (s *SMUnit) WithLaneSize(size int32) *SMUnit {
	s.meta.laneSize = size
	return s
}

func (s *SMUnit) WithALU(aluType string, num int32) *SMUnit {
	s.meta.alus = append(s.meta.alus, struct {
		aluType string
		aluNum  int32
	}{aluType: aluType, aluNum: num})
	return s
}

func (s *SMUnit) Build() {
	s.buildDispatcher()
	s.buildRegisterFile(s.meta.registerFileSize, s.meta.laneSize)
	s.aluGroup = make([]*alu.ALUGroup, len(s.meta.alus))
	for i, a := range s.meta.alus {
		s.aluGroup[i] = alu.NewALUGroup().WithALUType(a.aluType).WithALUNum(a.aluNum)
		s.aluGroup[i].Build()
	}
}

func (s *SMUnit) IsFree() bool {
	return true
}

func (s *SMUnit) Execute(warp *nvidia.Warp) {
	s.dispatcher.dispatch(warp)
}
