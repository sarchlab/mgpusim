package simulation

import (
	"bytes"
	"fmt"

	"github.com/sarchlab/akita/v5/timing"
)

// SaveCheckpoint writes a checkpoint archive for the simulation. The simulation
// must use a SerialEngine and be stopped outside an event handler. buildID
// overrides the build identity (mainly for tests); pass "" to use the default.
//
// The foundation milestone implements archive writing and validation only.
// Entity payload serializers land in later milestones, so this currently
// returns an error for the first entity that has no serializer.
func (s *Simulation) SaveCheckpoint(path, buildID string) error {
	if err := s.checkpointPreflight(); err != nil {
		return err
	}
	if buildID == "" {
		buildID = defaultBuildID()
	}

	payloads := make([]archiveEntry, 0, len(s.entities))
	for _, entity := range s.entities {
		checkpointable, ok := entity.(Checkpointable)
		if !ok {
			return fmt.Errorf(
				"checkpoint: entity %q (%T) has no checkpoint serializer",
				entity.Name(), entity)
		}

		var buf bytes.Buffer
		if err := checkpointable.SaveCheckpoint(&buf); err != nil {
			return fmt.Errorf("checkpoint: save entity %q: %w", entity.Name(), err)
		}
		payloads = append(payloads,
			archiveEntry{name: entity.Name(), data: buf.Bytes()})
	}

	return writeArchive(path, buildID, payloads)
}

// LoadCheckpoint loads a checkpoint archive into this rebuilt simulation. It
// checks the build identity and that the saved entity set matches the rebuilt
// one, then hands each entity its payload.
func (s *Simulation) LoadCheckpoint(path, buildID string) error {
	if err := s.checkpointPreflight(); err != nil {
		return err
	}
	if buildID == "" {
		buildID = defaultBuildID()
	}

	savedBuildID, payloads, err := readArchive(path)
	if err != nil {
		return err
	}
	if savedBuildID != buildID {
		return fmt.Errorf("checkpoint: build ID mismatch: checkpoint %q, current %q",
			savedBuildID, buildID)
	}
	if err := s.checkpointCoverage(payloads); err != nil {
		return err
	}

	for _, entity := range s.entities {
		checkpointable, ok := entity.(Checkpointable)
		if !ok {
			return fmt.Errorf(
				"checkpoint: entity %q (%T) has no checkpoint serializer",
				entity.Name(), entity)
		}

		if err := checkpointable.LoadCheckpoint(
			bytes.NewReader(payloads[entity.Name()]),
		); err != nil {
			return fmt.Errorf("checkpoint: load entity %q: %w", entity.Name(), err)
		}
	}

	return nil
}

func (s *Simulation) checkpointPreflight() error {
	if _, ok := s.engine.(*timing.SerialEngine); !ok {
		return fmt.Errorf("checkpoint: only timing.SerialEngine is supported, got %T",
			s.engine)
	}

	return nil
}

// checkpointCoverage verifies the saved entity set matches the rebuilt one.
func (s *Simulation) checkpointCoverage(payloads map[string][]byte) error {
	rebuilt := make(map[string]struct{}, len(s.entities))
	for _, entity := range s.entities {
		rebuilt[entity.Name()] = struct{}{}
	}

	for name := range payloads {
		if _, found := rebuilt[name]; !found {
			return fmt.Errorf("checkpoint: saved entity %q is not rebuilt", name)
		}
	}
	for name := range rebuilt {
		if _, found := payloads[name]; !found {
			return fmt.Errorf("checkpoint: rebuilt entity %q is missing from checkpoint",
				name)
		}
	}

	return nil
}
