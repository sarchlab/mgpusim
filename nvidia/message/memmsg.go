package message

import "github.com/sarchlab/akita/v4/sim"

type MemMsg struct {
	sim.MsgMeta
	MemAddress MemAddress
}

// Meta returns the meta data associated with the message.
func (m *MemMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone returns a clone of the MemMsg with different ID.
func (m *MemMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()

	return &cloneMsg
}

// MemMsgBuilder can build new CU restart reqs
type MemMsgBuilder struct {
	src, dst   sim.RemotePort
	memAddress MemAddress
}

// WithSrc sets the source of the request to build.
func (b MemMsgBuilder) WithSrc(src sim.RemotePort) MemMsgBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b MemMsgBuilder) WithDst(dst sim.RemotePort) MemMsgBuilder {
	b.dst = dst
	return b
}

type MemAddress struct {
	MemBaseAddr uint64
	MemOffset   uint64
}

func (b MemMsgBuilder) WithMemAddress(memAddress MemAddress) MemMsgBuilder {
	b.memAddress = memAddress
	return b
}

// Build creats a new MemMsg
func (b MemMsgBuilder) Build() *MemMsg {
	r := &MemMsg{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.MemAddress = b.memAddress
	return r
}
