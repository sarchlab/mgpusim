package tensor

import "testing"

func TestLocalReferencesRetireReplacedSlotsAndKeepAliases(t *testing.T) {
	o := CPUOperator{}
	r := NewReferences(o).(*localReferences)
	v := o.Create([]int{4})
	first, alias := r.Reference(v), r.Reference(v)
	first = r.Replace(first, o.Create([]int{4}))
	if r.Resolve(alias) != v {
		t.Fatal("replacing one alias retired another")
	}
	for i := 0; i < 100; i++ {
		first = r.Replace(first, o.Create([]int{4}))
	}
	if len(r.values) != 2 || len(r.ids) != 2 || len(r.counts) != 2 {
		t.Fatal("temporary bindings grew across replacements")
	}
	first = r.Replace(first, nil)
	alias = r.Replace(alias, nil)
	if first != 0 || alias != 0 || len(r.values) != 0 {
		t.Fatal("released bindings retained")
	}
}
