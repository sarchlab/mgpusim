package sm

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/smunit"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
)

type SMBuiler struct {
	parentNameString string
	counter          int32

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

func NewSMBuilder() *SMBuiler {
	return &SMBuiler{
		parentNameString: "",
		counter:          0,

		l1CacheSize:        0,
		smDispatchStrategy: "",
		smUnitCntPerSM:     0,

		l0CacheSize:          0,
		registerFileSize:     0,
		laneSize:             0,
		aluInt32CntPerSMUnit: 0,
	}
}

func (s *SMBuiler) WithParentNameString(parentNameString string) *SMBuiler {
	s.parentNameString = parentNameString
	return s
}

func (s *SMBuiler) WithSMUnitCnt(cnt int32) *SMBuiler {
	s.smUnitCntPerSM = cnt
	return s
}

func (s *SMBuiler) WithSMStrategy(strategy string) *SMBuiler {
	s.smDispatchStrategy = strategy
	return s
}

func (s *SMBuiler) WithL1CacheConfig(size int32) *SMBuiler {
	s.l1CacheSize = size
	return s
}

func (s *SMBuiler) WithL0CacheConfig(size int32) *SMBuiler {
	s.l0CacheSize = size
	return s
}

func (s *SMBuiler) WithRegisterFileConfig(registerFileSize int32, laneSize int32) *SMBuiler {
	s.registerFileSize = registerFileSize
	s.laneSize = laneSize
	return s
}

func (s *SMBuiler) WithALUConfig(aluType string, aluCnt int32) *SMBuiler {
	switch aluType {
	case "int32":
		s.aluInt32CntPerSMUnit = aluCnt
	default:
		panic("ALU type is not supported")
	}

	return s
}

func (s *SMBuiler) Build() *SM {
	sm := &SM{
		parentNameString: s.parentNameString,
		nameID:           fmt.Sprintf("%d", s.counter),
	}
	sm.dispatcher = s.buildDispatcher()

	sm.SMUnits = make([]runner.TraceableComponent, s.smUnitCntPerSM)
	smuBuilder := smunit.NewSMUnitBuilder().
		WithL0CacheConfig(s.l0CacheSize).
		WithRegisterFileConfig(s.registerFileSize, s.laneSize).
		WithALUConfig("int32", s.aluInt32CntPerSMUnit).
		WithParentNameString(sm.Name())
	for i := 0; i < int(s.smUnitCntPerSM); i++ {
		sm.SMUnits[i] = smuBuilder.Build()
	}

	s.counter++
	return sm
}
