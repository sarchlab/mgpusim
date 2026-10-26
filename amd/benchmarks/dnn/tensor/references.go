package tensor

// ID identifies a tensor in its operator's canonical state. Zero is nil.
type ID uint64

// References binds saved tensor handles to execution objects. GPU operators own
// the saved registry; CPU-only layers use a local, unsaved binding table.
type References interface {
	Reference(Tensor) ID
	Resolve(ID) Tensor
	Replace(ID, Tensor) ID
}

func NewReferences(to Operator) References {
	if r, ok := to.(References); ok {
		return r
	}
	return &localReferences{ids: make(map[Tensor]ID), values: make(map[ID]Tensor), counts: make(map[ID]int)}
}

type localReferences struct {
	ids    map[Tensor]ID
	values map[ID]Tensor
	counts map[ID]int
	next   ID
}

func (r *localReferences) Reference(t Tensor) ID {
	if t == nil {
		return 0
	}
	if id := r.ids[t]; id != 0 {
		r.counts[id]++
		return id
	}
	r.next++
	r.ids[t], r.values[r.next] = r.next, t
	r.counts[r.next] = 1
	return r.next
}
func (r *localReferences) Resolve(id ID) Tensor { return r.values[id] }

// Replace releases a local binding when an execution field is overwritten.
func (r *localReferences) Replace(old ID, t Tensor) ID {
	if old != 0 && r.values[old] == t {
		return old
	}
	next := r.Reference(t)
	if old != 0 {
		r.counts[old]--
		if r.counts[old] == 0 {
			delete(r.ids, r.values[old])
			delete(r.values, old)
			delete(r.counts, old)
		}
	}
	return next
}
