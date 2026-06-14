package timing

import (
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
)

// idGeneratorCheckpoint is the serialized form of the ID generator: its kind and
// next counter value.
type idGeneratorCheckpoint struct {
	Kind   string `json:"kind"`
	NextID uint64 `json:"next_id"`
}

// SaveCheckpoint writes the sequential ID generator's kind and next counter.
func (g *sequentialIDGenerator) SaveCheckpoint(w io.Writer) error {
	dto := idGeneratorCheckpoint{
		Kind:   "sequential",
		NextID: atomic.LoadUint64(&g.nextID),
	}
	return json.NewEncoder(w).Encode(dto)
}

// LoadCheckpoint restores the sequential ID generator's counter.
func (g *sequentialIDGenerator) LoadCheckpoint(r io.Reader) error {
	var dto idGeneratorCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return err
	}
	if dto.Kind != "sequential" {
		return fmt.Errorf(
			"timing: ID generator kind mismatch: checkpoint %q, rebuilt sequential",
			dto.Kind)
	}
	atomic.StoreUint64(&g.nextID, dto.NextID)
	return nil
}

// SaveCheckpoint rejects checkpointing the parallel ID generator: its IDs are not
// deterministic, so a restored counter would not reproduce the same IDs.
func (g *parallelIDGenerator) SaveCheckpoint(_ io.Writer) error {
	return fmt.Errorf("timing: parallel ID generator is not checkpointable")
}

// LoadCheckpoint rejects loading into the parallel ID generator.
func (g *parallelIDGenerator) LoadCheckpoint(_ io.Reader) error {
	return fmt.Errorf("timing: parallel ID generator is not checkpointable")
}
