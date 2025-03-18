package nvidiaconfig

type ExecType int16

const (
	ExecUndefined ExecType = iota
	ExecKernel
	ExecMemcpy
)

type ExecMemcpyDirection string

const (
	ExecMemcpyDirectionUndefined ExecMemcpyDirection = ""
	H2D                          ExecMemcpyDirection = "MemcpyHtoD"
	D2H                          ExecMemcpyDirection = "MemcpyDtoH"
)
