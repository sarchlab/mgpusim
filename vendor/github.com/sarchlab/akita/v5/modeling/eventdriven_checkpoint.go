package modeling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/timing"
)

// eventDrivenCheckpoint is the serialized form of an event-driven component: a
// spec hash for compatibility checking, the mutable State, and the wakeup guard.
// Ports and resources are rebuilt by setup, not serialized. The guard
// (pendingWakeup) is saved directly — alongside the engine's matching timer
// event — so load is a single pass with no post-load reconciliation.
type eventDrivenCheckpoint struct {
	SpecHash      string                `json:"spec_hash"`
	State         json.RawMessage       `json:"state"`
	PendingWakeup timing.VTimeInPicoSec `json:"pending_wakeup"`
}

// SaveCheckpoint writes the component's spec hash, State, and wakeup guard as
// JSON. It implements the structural Checkpointable contract without importing
// the simulation package.
func (c *EventDrivenComponent[S, T, R]) SaveCheckpoint(w io.Writer) error {
	state, err := json.Marshal(c.State)
	if err != nil {
		return fmt.Errorf("modeling: marshal state: %w", err)
	}

	c.Lock()
	pendingWakeup := c.pendingWakeup
	c.Unlock()

	return json.NewEncoder(w).Encode(eventDrivenCheckpoint{
		SpecHash:      c.specHash(),
		State:         state,
		PendingWakeup: pendingWakeup,
	})
}

// LoadCheckpoint restores State and the wakeup guard after verifying that the
// saved spec hash matches the rebuilt component's.
func (c *EventDrivenComponent[S, T, R]) LoadCheckpoint(r io.Reader) error {
	var dto eventDrivenCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("modeling: decode event-driven checkpoint: %w", err)
	}

	if got := c.specHash(); got != dto.SpecHash {
		return fmt.Errorf(
			"modeling: spec hash mismatch: checkpoint %s, rebuilt %s",
			dto.SpecHash, got)
	}

	var state T
	if err := json.Unmarshal(dto.State, &state); err != nil {
		return fmt.Errorf("modeling: unmarshal state: %w", err)
	}

	c.Lock()
	c.State = state
	c.pendingWakeup = dto.PendingWakeup
	c.Unlock()

	return nil
}

// specHash is a deterministic fingerprint of the component's immutable Spec, used
// to reject loading a checkpoint into a component built with a different config.
func (c *EventDrivenComponent[S, T, R]) specHash() string {
	data, err := json.Marshal(c.spec)
	if err != nil {
		panic(fmt.Sprintf("modeling: cannot hash spec: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
