package messaging

import "github.com/sarchlab/akita/v5/internal/codec"

// msgCodec decodes the polymorphic messages held in port buffers across a
// checkpoint. Each concrete message type is registered with RegisterMsg; the
// wire format and reflection machinery live in package codec.
var msgCodec = codec.NewRegistry[Msg]("message")

// RegisterMsg registers a concrete message type so a checkpoint that captured it
// in a port buffer can be decoded. It is the low-level primitive behind
// DefineProtocol, which is the recommended way to register messages: a protocol
// definition registers every message type it carries in one declaration.
// Messages are value types, so register the value (a pointer also works). The
// tag is derived from the Go type, so checkpoints are restored by the same
// binary. Registering the same type twice is harmless.
//
// A forgotten registration fails loudly at load time, not silently: decoding a
// checkpoint that holds an unregistered message reports an unknown-message-type
// error.
func RegisterMsg(msg Msg) {
	msgCodec.Register(msg)
}
