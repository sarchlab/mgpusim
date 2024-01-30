package smunit

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/alu"
)

type SMUnitBuilder struct {
	l0CacheSize          int32
	registerFileSize     int32
	laneSize             int32
	aluInt32CntPerSMUnit int32
}

func NewSMUnitBuilder() *SMUnitBuilder {
	return &SMUnitBuilder{
		l0CacheSize:          0,
		registerFileSize:     0,
		laneSize:             0,
		aluInt32CntPerSMUnit: 0,
	}
}

func (s *SMUnitBuilder) WithL0CacheConfig(size int32) *SMUnitBuilder {
	s.l0CacheSize = size
	return s
}

func (s *SMUnitBuilder) WithRegisterFileConfig(registerFileSize int32, laneSize int32) *SMUnitBuilder {
	s.registerFileSize = registerFileSize
	s.laneSize = laneSize
	return s
}

func (s *SMUnitBuilder) WithALUConfig(aluType string, cntPerSMUnit int32) *SMUnitBuilder {
	switch aluType {
	case "int32":
		s.aluInt32CntPerSMUnit = cntPerSMUnit
	default:
		panic("ALU type is not supported")
	}

	return s
}

func (s *SMUnitBuilder) Build() *SMUnit {
	smu := new(SMUnit)
	smu.registerFile = s.buildRegisterFile()
	smu.aluInt32 = make([]alu.ALU, s.aluInt32CntPerSMUnit)
	for i := range smu.aluInt32 {
		smu.aluInt32[i] = alu.NewALUBuilder().
			WithALUType("int32").
			Build()
	}
	
	return smu
}

func (s *SMUnitBuilder) buildRegisterFile() *RegisterFile {
	rf := new(RegisterFile)
	rf.buf = make([]byte, s.registerFileSize)
	rf.byteSizePerLane = s.laneSize
	return rf
}
