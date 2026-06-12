package simulation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
)

// StateSaver is implemented by components that can save their state to a
// writer. modeling.Component[S,T] satisfies this via its SaveState method.
type StateSaver interface {
	SaveState(w io.Writer) error
}

// StateLoader is implemented by components that can load their state from a
// reader. modeling.Component[S,T] satisfies this via its LoadState method.
type StateLoader interface {
	LoadState(r io.Reader) error
}

// StorageOwner is implemented by components that own a mem.Storage that
// should be checkpointed.
type StorageOwner interface {
	GetStorage() *mem.Storage
	StorageName() string
}

// TickResetter is implemented by components that can reset their tick
// scheduler after loading. This only resets the internal guard so that
// future TickLater calls can schedule new events. It does NOT schedule
// a tick by itself — ticks are triggered naturally via port notifications
// or explicit TickLater calls by the user.
type TickResetter interface {
	ResetTick()
}

// WakeupResetter is implemented by event-driven components that need to
// reset their pending wakeup guard after loading state from a checkpoint.
type WakeupResetter interface {
	ResetWakeup()
}

// checkpointMetadata is the top-level metadata saved with a checkpoint.
type checkpointMetadata struct {
	EngineTime      sim.VTimeInSec `json:"engine_time"`
	IDGeneratorNext uint64         `json:"id_generator_next"`
}

// Save persists the simulation state to the given directory path.
//
// All port buffers must be empty (quiescence) before saving. The event queue
// is NOT serialized — it will be reconstructed via TickLater on Load.
// Connections are NOT serialized — they are reconstructed from build code.
func (s *Simulation) Save(path string) error {
	if err := s.verifyQuiescence(); err != nil {
		return fmt.Errorf("cannot save: %w", err)
	}

	if err := s.createCheckpointDirs(path); err != nil {
		return err
	}

	if err := s.saveMetadata(path); err != nil {
		return err
	}

	if err := s.saveComponentStates(filepath.Join(path, "components")); err != nil {
		return err
	}

	if err := s.saveStorages(filepath.Join(path, "storage")); err != nil {
		return err
	}

	return nil
}

func (s *Simulation) createCheckpointDirs(path string) error {
	if err := os.MkdirAll(filepath.Join(path, "components"), 0o755); err != nil {
		return fmt.Errorf("create component dir: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(path, "storage"), 0o755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	return nil
}

func (s *Simulation) saveMetadata(path string) error {
	meta := checkpointMetadata{
		EngineTime:      s.engine.CurrentTime(),
		IDGeneratorNext: sim.GetIDGeneratorNextID(),
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(filepath.Join(path, "metadata.json"), metaData, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return nil
}

func (s *Simulation) saveComponentStates(compDir string) error {
	for _, comp := range s.components {
		saver, ok := comp.(StateSaver)
		if !ok {
			continue
		}

		filePath := filepath.Join(compDir, comp.Name()+".json")

		f, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("create component file %s: %w", comp.Name(), err)
		}

		if err := saver.SaveState(f); err != nil {
			f.Close()
			return fmt.Errorf("save state for %s: %w", comp.Name(), err)
		}

		f.Close()
	}

	return nil
}

func (s *Simulation) saveStorages(storageDir string) error {
	for _, comp := range s.components {
		owner, ok := comp.(StorageOwner)
		if !ok {
			continue
		}

		storage := owner.GetStorage()
		if storage == nil {
			continue
		}

		name := owner.StorageName()
		filePath := filepath.Join(storageDir, name+".bin")

		f, err := os.Create(filePath)
		if err != nil {
			return fmt.Errorf("create storage file %s: %w", name, err)
		}

		if err := storage.Save(f); err != nil {
			f.Close()
			return fmt.Errorf("save storage %s: %w", name, err)
		}

		f.Close()
	}

	return nil
}

// Load restores simulation state from the given directory path.
//
// The simulation must already be built (topology and connections reconstructed
// from build code) before calling Load. After loading, TickResetter components
// will have their tick schedulers reset and a new tick scheduled.
func (s *Simulation) Load(path string) error {
	if err := s.loadMetadata(path); err != nil {
		return err
	}

	if err := s.loadComponentStates(filepath.Join(path, "components")); err != nil {
		return err
	}

	if err := s.loadStorages(filepath.Join(path, "storage")); err != nil {
		return err
	}

	s.resetTickSchedulers()

	return nil
}

func (s *Simulation) loadMetadata(path string) error {
	metaData, err := os.ReadFile(filepath.Join(path, "metadata.json"))
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}

	var meta checkpointMetadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("unmarshal metadata: %w", err)
	}

	se, ok := s.engine.(*sim.SerialEngine)
	if !ok {
		return fmt.Errorf("Load requires SerialEngine")
	}

	se.SetCurrentTime(meta.EngineTime)
	sim.SetIDGeneratorNextID(meta.IDGeneratorNext)

	return nil
}

func (s *Simulation) loadComponentStates(compDir string) error {
	for _, comp := range s.components {
		loader, ok := comp.(StateLoader)
		if !ok {
			continue
		}

		filePath := filepath.Join(compDir, comp.Name()+".json")

		f, err := os.Open(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("open component file %s: %w", comp.Name(), err)
		}

		if err := loader.LoadState(f); err != nil {
			f.Close()
			return fmt.Errorf("load state for %s: %w", comp.Name(), err)
		}

		f.Close()
	}

	return nil
}

func (s *Simulation) loadStorages(storageDir string) error {
	for _, comp := range s.components {
		owner, ok := comp.(StorageOwner)
		if !ok {
			continue
		}

		storage := owner.GetStorage()
		if storage == nil {
			continue
		}

		name := owner.StorageName()
		filePath := filepath.Join(storageDir, name+".bin")

		f, err := os.Open(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("open storage file %s: %w", name, err)
		}

		if err := storage.Load(f); err != nil {
			f.Close()
			return fmt.Errorf("load storage %s: %w", name, err)
		}

		f.Close()
	}

	return nil
}

// resetTickSchedulers resets tick schedulers so future TickLater calls can
// schedule events. We only reset the scheduler guard — we do NOT auto-schedule
// ticks. Components will be ticked naturally when ports receive messages or
// when the caller explicitly calls TickLater on specific components.
func (s *Simulation) resetTickSchedulers() {
	for _, comp := range s.components {
		if resetter, ok := comp.(TickResetter); ok {
			resetter.ResetTick()
		}

		if resetter, ok := comp.(WakeupResetter); ok {
			resetter.ResetWakeup()
		}
	}
}

// verifyQuiescence checks that all registered ports have empty buffers.
func (s *Simulation) verifyQuiescence() error {
	for _, p := range s.ports {
		if p.NumIncoming() != 0 {
			return fmt.Errorf("port %s has %d incoming messages",
				p.Name(), p.NumIncoming())
		}

		if p.NumOutgoing() != 0 {
			return fmt.Errorf("port %s has %d outgoing messages",
				p.Name(), p.NumOutgoing())
		}
	}

	return nil
}
