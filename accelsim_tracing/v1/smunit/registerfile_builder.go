package smunit

import "github.com/sarchlab/mgpusim/v3/samples/runner"

type RegisterFileBuilder struct {
	parentNameString string

	size       int32
	rfLaneSize int32
}

func NewRegisterFileBuilder() *RegisterFileBuilder {
	return &RegisterFileBuilder{
		parentNameString: "",
		size:             0,
		rfLaneSize:       0,
	}
}

func (r *RegisterFileBuilder) WithParentNameString(parentNameString string) *RegisterFileBuilder {
	r.parentNameString = parentNameString
	return r
}

func (r *RegisterFileBuilder) WithSize(size int32) *RegisterFileBuilder {
	r.size = size
	return r
}

func (r *RegisterFileBuilder) WithLaneSize(laneSize int32) *RegisterFileBuilder {
	r.rfLaneSize = laneSize
	return r
}

func (r *RegisterFileBuilder) Build() runner.TraceableComponent {
	rf := &RegisterFile{
		parentNameString: r.parentNameString,
		size:             r.size,
		rfLaneSize:       r.rfLaneSize,
	}
	rf.buf = make([]byte, r.size)
	rf.byteSizePerLane = r.size / r.rfLaneSize
	return rf
}
