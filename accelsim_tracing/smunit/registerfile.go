package smunit

type RegisterFile struct {
	RfSize          int32
	rfLaneSize      int32
	buf             []byte
	byteSizePerLane int32
}

func (r *RegisterFile) Read(offset int32, width int32) {
}

func (r *RegisterFile) Write(offset int32, width int32) {
}

func (s *SMUnit) buildRegisterFile(size int32, sizePerLane int32) {
	s.registerFile = &RegisterFile{
		buf:             make([]byte, size),
		byteSizePerLane: sizePerLane,
	}
}
