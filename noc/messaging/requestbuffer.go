package messaging

import "github.com/sarchlab/akita/v3/sim"

// MsgBuffer is a buffer that can hold requests
type MsgBuffer struct {
	Capacity int
	Buf      []sim.Msg
	vc       int
}

func (b *MsgBuffer) enqueue(req sim.Msg) {
	if len(b.Buf) > b.Capacity {
		panic("buffer overflow")
	}

	b.Buf = append(b.Buf, req)
}
