package queueing

import "encoding/json"

// pipelineState is the JSON form of a Pipeline: its geometry and the items
// currently occupying it. Pipeline's fields are unexported, so without these
// methods encoding/json would serialize a Pipeline as an empty object and
// silently drop the in-flight items. PipelineStage is already all-exported, so
// the stages serialize directly.
type pipelineState[T any] struct {
	Width     int                `json:"width"`
	NumStages int                `json:"num_stages"`
	Stages    []PipelineStage[T] `json:"stages"`
}

// MarshalJSON serializes the pipeline's geometry and occupied stages.
func (p Pipeline[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(pipelineState[T]{
		Width:     p.width,
		NumStages: p.numStages,
		Stages:    p.stages,
	})
}

// UnmarshalJSON restores a pipeline from its JSON form.
func (p *Pipeline[T]) UnmarshalJSON(data []byte) error {
	var s pipelineState[T]
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	p.width = s.Width
	p.numStages = s.NumStages
	p.stages = s.Stages

	return nil
}
