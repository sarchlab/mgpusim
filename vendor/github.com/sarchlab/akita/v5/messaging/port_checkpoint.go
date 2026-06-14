package messaging

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/queueing"
)

// bufferCheckpoint is the serialized form of one port buffer: its capacity (a
// shape check — compared on load, never restored) together with its in-flight
// messages, encoded through the message codec so their concrete types survive.
// Bundling capacity with the contents keeps each buffer one self-contained
// element, reflecting that the capacity is the buffer's own immutable config.
type bufferCheckpoint struct {
	Capacity int             `json:"capacity"`
	Elements json.RawMessage `json:"elements"`
}

// portCheckpoint is the serialized form of a port: one self-contained element per
// buffer. Hooks and the owning component/connection are rebuilt by setup and not
// serialized.
type portCheckpoint struct {
	Incoming bufferCheckpoint `json:"incoming"`
	Outgoing bufferCheckpoint `json:"outgoing"`
}

// SaveCheckpoint writes the port's two buffers (capacity plus contents). Message
// types must be registered with RegisterMsg.
func (p *defaultPort) SaveCheckpoint(w io.Writer) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	incoming, err := saveBuffer(&p.incomingBuf, p.name, "incoming")
	if err != nil {
		return err
	}
	outgoing, err := saveBuffer(&p.outgoingBuf, p.name, "outgoing")
	if err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(portCheckpoint{
		Incoming: incoming,
		Outgoing: outgoing,
	})
}

// LoadCheckpoint restores the buffer contents after checking that each rebuilt
// buffer has the saved capacity. Buffers are restored directly, without calling
// Send/Deliver, so no hooks fire and no connection is notified.
func (p *defaultPort) LoadCheckpoint(r io.Reader) error {
	var dto portCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	if err := loadBuffer(&p.incomingBuf, dto.Incoming, p.name, "incoming"); err != nil {
		return err
	}

	return loadBuffer(&p.outgoingBuf, dto.Outgoing, p.name, "outgoing")
}

// saveBuffer captures a buffer's capacity and its type-tagged contents.
func saveBuffer(
	buf *queueing.Buffer[Msg], portName, label string,
) (bufferCheckpoint, error) {
	elements, err := msgCodec.EncodeSlice(buf.Elements())
	if err != nil {
		return bufferCheckpoint{}, fmt.Errorf(
			"messaging: port %q %s: %w", portName, label, err)
	}

	return bufferCheckpoint{
		Capacity: buf.Capacity(),
		Elements: elements,
	}, nil
}

// loadBuffer checks the rebuilt buffer's capacity against the checkpoint (a shape
// check) and restores its contents directly, firing no hooks.
func loadBuffer(
	buf *queueing.Buffer[Msg], bc bufferCheckpoint, portName, label string,
) error {
	if got := buf.Capacity(); bc.Capacity != got {
		return fmt.Errorf(
			"messaging: port %q %s capacity mismatch: checkpoint %d, rebuilt %d",
			portName, label, bc.Capacity, got)
	}

	elements, err := msgCodec.DecodeSlice(bc.Elements)
	if err != nil {
		return fmt.Errorf("messaging: port %q %s: %w", portName, label, err)
	}

	buf.Restore(elements)

	return nil
}
