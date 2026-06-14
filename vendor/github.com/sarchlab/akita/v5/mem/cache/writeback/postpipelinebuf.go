package writeback

// postPipelineBuf is a bounded, ordered collection of transaction indices that
// have left a bank pipeline and await finalization. Unlike a FIFO, a finalized
// transaction can be removed from any position: the front item may be blocked
// on port backpressure while a later item completes. It satisfies
// queueing.Sink[int] so a pipeline can push completed items into it.
type postPipelineBuf struct {
	Cap   int   `json:"cap"`
	Items []int `json:"items"`
}

func newPostPipelineBuf(capacity int) postPipelineBuf {
	return postPipelineBuf{Cap: capacity}
}

// CanPush reports whether the buffer has room for another item.
func (b *postPipelineBuf) CanPush() bool {
	return len(b.Items) < b.Cap
}

// PushTyped appends an item to the back of the buffer.
func (b *postPipelineBuf) PushTyped(item int) {
	b.Items = append(b.Items, item)
}

// Size returns the number of items in the buffer.
func (b *postPipelineBuf) Size() int {
	return len(b.Items)
}

// Get returns the item at position i.
func (b *postPipelineBuf) Get(i int) int {
	return b.Items[i]
}

// RemoveAt removes the item at position i, preserving the order of the rest.
func (b *postPipelineBuf) RemoveAt(i int) {
	b.Items = append(b.Items[:i], b.Items[i+1:]...)
}

// Clear removes all items from the buffer.
func (b *postPipelineBuf) Clear() {
	b.Items = nil
}
