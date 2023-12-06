package sm

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/smunit"
)

type SM struct {
	meta       *smMetaData
	dispatcher smDispatcher
	smUnits    []*smunit.SMUnit
}

type smMetaData struct {
	smUnitNum int32

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

func NewSM() *SM {
	return &SM{
		meta: &smMetaData{
			smUnitNum: 0,

			smStrategy:     "default",
			smUnitStrategy: "default",

			l1CacheSize: 0,
			l0CacheSize: 0,

			registerFileSize: 0,
			laneSize:         0,

			alus: nil,
		},
		dispatcher: nil,
		smUnits:    nil,
	}
}

func (s *SM) WithSMStrategy(strategy string) *SM {
	s.meta.smStrategy = strategy
	return s
}

func (s *SM) WithSMUnitNum(num int32) *SM {
	s.meta.smUnitNum = num
	return s
}

func (s *SM) WithSMUnitStrategy(strategy string) *SM {
	s.meta.smUnitStrategy = strategy
	return s
}

func (s *SM) WithL1CacheSize(size int32) *SM {
	s.meta.l1CacheSize = size
	return s
}

func (s *SM) WithL0CacheSize(size int32) *SM {
	s.meta.l0CacheSize = size
	return s
}

func (s *SM) WithRegisterFileSize(size int32) *SM {
	s.meta.registerFileSize = size
	return s
}

func (s *SM) WithLaneSize(size int32) *SM {
	s.meta.laneSize = size
	return s
}

func (s *SM) WithALU(aluType string, aluNum int32) *SM {
	s.meta.alus = append(s.meta.alus, struct {
		aluType string
		aluNum  int32
	}{aluType: aluType, aluNum: aluNum})
	return s
}

func (s *SM) Build() {
	s.buildDispatcher()
	s.smUnits = make([]*smunit.SMUnit, s.meta.smUnitNum)
	for i := 0; i < int(s.meta.smUnitNum); i++ {
		s.smUnits[i] = smunit.NewSMUnit().WithSMUnitStrategy(s.meta.smUnitStrategy).
			WithL0CacheSize(s.meta.l0CacheSize).
			WithRegisterFileSize(s.meta.registerFileSize).WithLaneSize(s.meta.laneSize)
		for _, alu := range s.meta.alus {
			s.smUnits[i].WithALU(alu.aluType, alu.aluNum)
		}
		s.smUnits[i].Build()
	}
}

func (s *SM) IsFree() bool {
	return true
}

func (s *SM) Execute(tb *nvidia.ThreadBlock) {
	s.dispatcher.dispatch(tb)
}
