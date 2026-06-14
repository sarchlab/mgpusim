package messaging

// Msg is the interface for all messages transferred between components.
//
// Messages are value types: a message is constructed as a struct value (e.g.
// `memprotocol.ReadReq{...}`) and passed by value through ports. Once a message has
// been handed to a port, it is single-use, immutable data — callers must not
// mutate the value after Send/Deliver.
type Msg interface {
	Meta() MsgMeta
}

// MsgMeta contains routing and identification metadata. All fields are set at
// construction time and must not change once the message is in flight.
type MsgMeta struct {
	ID           uint64
	Src, Dst     RemotePort
	TrafficClass string
	TrafficBytes int
	RspTo        uint64
}

// Meta returns the message metadata.
func (m MsgMeta) Meta() MsgMeta { return m }

// IsRsp returns true if this message is a response to another message.
func (m MsgMeta) IsRsp() bool { return m.RspTo != 0 }
