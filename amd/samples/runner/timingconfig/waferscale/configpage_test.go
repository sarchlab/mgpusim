package waferscale

import (
	"reflect"
	"testing"
)

func TestFlattenJSONKeepsFieldOrder(t *testing.T) {
	fields, err := flattenJSON([]byte(
		`{"freq":1000,"bottom":{"destinations":["A.Top","B.Top"],"size":7},"name":"x","none":[],"nil":null}`))
	if err != nil {
		t.Fatal(err)
	}

	want := []Field{
		{"freq", "1000"},
		{"bottom.destinations[0]", `"A.Top"`},
		{"bottom.destinations[1]", `"B.Top"`},
		{"bottom.size", "7"},
		{"name", `"x"`},
		{"none", "[]"},
		{"nil", "null"},
	}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("got %v, want %v", fields, want)
	}
}

func TestSplitSeparatesCommonFields(t *testing.T) {
	p := pendingGroup{
		names: []string{"CU[0]", "CU[1]"},
		fields: [][]Field{
			{{"freq", "1"}, {"to", `"ROB[0]"`}},
			{{"freq", "1"}, {"to", `"ROB[1]"`}},
		},
	}

	g := p.split("CU[]")
	if !reflect.DeepEqual(g.Common, []Field{{"freq", "1"}}) ||
		!reflect.DeepEqual(g.Differing, []string{"to"}) ||
		!reflect.DeepEqual(g.Values, [][]string{{`"ROB[0]"`}, {`"ROB[1]"`}}) {
		t.Fatalf("unexpected split %+v", g)
	}
}
