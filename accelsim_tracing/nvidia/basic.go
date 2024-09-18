package nvidia

const (
	BYTE  = 8
	WORD  = 16
	DWORD = 32
)

type Dim3 [3]int32

func (d Dim3) X() int32 {
	return d[0]
}

func (d Dim3) Y() int32 {
	return d[1]
}

func (d Dim3) Z() int32 {
	return d[2]
}
