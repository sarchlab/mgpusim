package smunit

type RegisterFile struct {
	size            int32
	rfLaneSize      int32
	buf             []byte
	byteSizePerLane int32
}

func (r *RegisterFile) Read(offset int32, width int32) {
}

func (r *RegisterFile) Write(offset int32, width int32) {
}
