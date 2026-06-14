package queueing

import "encoding/json"

// bufferState is the JSON form of a Buffer: its name, capacity, and FIFO
// contents. Buffer's fields are unexported, so without these methods
// encoding/json would serialize a Buffer as an empty object and silently drop
// its contents. The MarshalJSON receiver is a value (not a pointer) so it is
// invoked even when a Buffer is embedded by value in a component's State.
type bufferState[T any] struct {
	Name     string `json:"name"`
	Cap      int    `json:"cap"`
	Elements []T    `json:"elements"`
}

// MarshalJSON serializes the buffer's name, capacity, and elements.
func (b Buffer[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(bufferState[T]{
		Name:     b.name,
		Cap:      b.cap,
		Elements: b.elements,
	})
}

// UnmarshalJSON restores a buffer from its JSON form.
func (b *Buffer[T]) UnmarshalJSON(data []byte) error {
	var s bufferState[T]
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	b.name = s.Name
	b.cap = s.Cap
	b.elements = s.Elements

	return nil
}
