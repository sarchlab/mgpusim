package sim

// Msg is the interface for all messages transferred between components.
type Msg interface {
	Meta() *MsgMeta
}

// MsgMeta contains routing and identification metadata.
type MsgMeta struct {
	ID           uint64
	Src, Dst     RemotePort
	TrafficClass string
	TrafficBytes int
	RspTo        uint64
	SendTaskID   uint64 `json:"send_task_id"`
	RecvTaskID   uint64 `json:"recv_task_id"`
}

// Meta returns the message metadata.
func (m *MsgMeta) Meta() *MsgMeta { return m }

// IsRsp returns true if this message is a response to another message.
func (m *MsgMeta) IsRsp() bool { return m.RspTo != 0 }
