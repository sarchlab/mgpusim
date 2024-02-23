package smunit

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/alu"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/v3/samples/runner"
	"gitlab.com/akita/akita"
)

type SMUnitBuilder struct {
	parentNameString string
	counter          int32

	l0CacheSize int32

	registerFileSize int32
	laneSize         int32

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

func (s *SMUnitBuilder) WithParentNameString(parentNameString string) *SMUnitBuilder {
	s.parentNameString = parentNameString
	return s
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

func (s *SMUnitBuilder) Build() runner.TraceableComponent {
	smu := &SMUnit{
		parentNameString: s.parentNameString,
		nameID:           fmt.Sprintf("%d", s.counter),
	}

	rfBuiler := NewRegisterFileBuilder().
		WithLaneSize(s.laneSize).
		WithSize(s.registerFileSize).
		WithParentNameString(smu.Name())
	smu.RegisterFile = rfBuiler.Build()

	smu.ALUInt32 = make([]runner.TraceableComponent, s.aluInt32CntPerSMUnit)
	aluBuilder := alu.NewALUBuilder().
		WithParentNameString(smu.Name())
	for i := range smu.ALUInt32 {
		smu.ALUInt32[i] = aluBuilder.Build(nvidia.ALUINT32)
		smu.ALUInt32Conn[i] = akita.NewDirectConnection("conn", )
	}

	s.counter++
	return smu
}
