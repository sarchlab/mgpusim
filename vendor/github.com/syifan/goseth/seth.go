// Package goseth defines a serializer for Go data structures.
package goseth

import "io"

// Serializer can serialize anything into a json format.
type Serializer interface {
	// SetRoot sets the item to be serialized.
	SetRoot(item any)

	// SetMaxDepth sets the maximum depth of the serialization. If the depth
	// is exceeded, the serializer will stop serializing the item.
	// SetMaxDepth(-1) means no limit.
	SetMaxDepth(depth int)

	// SetEntryPoint sets the entry point of the serialization. If this
	// function is not called, the entry point is the root item. Otherwise, the
	// serializer will first drill down to the specific point before
	// serializing. For example, suppose we have a struct called Foo that has a
	// field called Bar. If we set the root to be Foo and set the entry point to
	// be []string{"Bar"}, the serializer will only serialize the Bar field.
	SetEntryPoint(entryPoint []string) error

	// Serialize serializes the item into the writer.
	Serialize(writer io.Writer) error
}

// NewSerializer returns a new default serializer.
func NewSerializer() Serializer {
	return &serializer{maxDepth: -1}
}
