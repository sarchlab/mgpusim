package driver

import (
	"bytes"
	"encoding/json"

	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// These test-only records inspect saved public checkpoint bytes. They neither
// expose Runtime's live state nor add test hooks to the production API.
type savedRuntime struct {
	NextCommand gpuruntime.CommandID `json:"NextCommand"`
	Queues      []struct {
		Context    gpuruntime.Context   `json:"Context"`
		GPU        int                  `json:"GPU"`
		Pending    []gpuruntime.Command `json:"Pending"`
		Unfinished uint64               `json:"Unfinished"`
	} `json:"Queues"`
	Contexts []struct {
		GPU     int `json:"GPU"`
		Buffers []struct {
			Address gpuruntime.Ptr `json:"Address"`
			Size    uint64         `json:"Size"`
			Dirty   bool           `json:"Dirty"`
		} `json:"Buffers"`
	} `json:"Contexts"`
	Events map[gpuruntime.Event]json.RawMessage `json:"Events"`
}

func (f *fixture) savedEnvelope() map[string]json.RawMessage {
	var b bytes.Buffer
	if err := f.r.SaveCheckpoint(&b); err != nil {
		panic(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(b.Bytes(), &envelope); err != nil {
		panic(err)
	}
	return envelope
}

//nolint:musttag // Inspect the existing untagged Command checkpoint schema without duplicating it.
func (f *fixture) snapshot() savedRuntime {
	envelope := f.savedEnvelope()
	var payload struct {
		State savedRuntime `json:"State"`
	}
	if err := json.Unmarshal(envelope["state"], &payload); err != nil {
		panic(err)
	}
	return payload.State
}

// enqueue builds a checkpoint fixture containing a synthetic protocol command.
// Real callers submit through Runtime's typed host methods.
func (f *fixture) enqueue(q gpuruntime.CommandQueue, cmd gpuruntime.Command) gpuruntime.CommandID {
	envelope := f.savedEnvelope()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(envelope["state"], &payload); err != nil {
		panic(err)
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(payload["State"], &state); err != nil {
		panic(err)
	}
	snapshot := f.snapshot()
	snapshot.NextCommand++
	cmd.ID = snapshot.NextCommand
	queue := &snapshot.Queues[q-1]
	queue.Pending = append(queue.Pending, cmd)
	queue.Unfinished++
	state["NextCommand"] = marshalFixture(snapshot.NextCommand)
	state["Queues"] = marshalFixture(snapshot.Queues)
	payload["State"] = marshalFixture(state)
	envelope["state"] = marshalFixture(payload)
	data, err := json.Marshal(envelope)
	if err != nil {
		panic(err)
	}
	if err := f.r.LoadCheckpoint(bytes.NewReader(data)); err != nil {
		panic(err)
	}
	return cmd.ID
}

func marshalFixture(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
