package driver

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestRestoredResultBuffers(t *testing.T) {
	pending := &MemCopyD2HCommand{ID: 17, ByteSize: 8}
	d := &Driver{
		contexts:       []*Context{{queues: []*CommandQueue{{commands: []Command{pending}}}}},
		unboundResults: map[uint64][]byte{23: {7, 0, 0, 0, 9, 0, 0, 0}},
	}
	want := []ResultBuffer{{CommandID: 17, ByteSize: 8}, {CommandID: 23, ByteSize: 8, Complete: true}}
	if got := d.ResultBuffers(); !reflect.DeepEqual(got, want) {
		t.Fatalf("results: %+v", got)
	}
	for _, id := range []uint64{17, 23} {
		for _, invalid := range []any{nil, []uint32{1}, uint64(1), (*uint64)(nil)} {
			if err := d.BindResultBuffer(id, invalid); err == nil {
				t.Fatalf("accepted invalid destination %T", invalid)
			}
		}
	}
	output := []uint32{101, 103}
	if err := d.BindResultBuffer(17, &output); err != nil {
		t.Fatal(err)
	}
	if output[0] != 101 || output[1] != 103 {
		t.Fatal("binding changed pending output")
	}
	pending.RawData = []byte{3, 0, 0, 0, 5, 0, 0, 0}
	d.deliverResult(pending)
	if output[0] != 3 || output[1] != 5 {
		t.Fatalf("pending delivery: %v", output)
	}
	completed := make([]uint32, 2)
	if err := d.BindResultBuffer(23, completed); err != nil {
		t.Fatal(err)
	}
	if completed[0] != 7 || completed[1] != 9 {
		t.Fatalf("completed delivery: %v", completed)
	}
	if len(d.ResultBuffers()) != 0 {
		t.Fatal("bound copies still reported")
	}
	if err := d.BindResultBuffer(23, completed); err == nil {
		t.Fatal("delivered completed result twice")
	}
}

func TestHostCopySnapshotOwnsBytes(t *testing.T) {
	source := []uint32{11, 13}
	saved, err := snapshotCommand(&MemCopyH2DCommand{ID: 2, Src: source, Dst: 4096})
	if err != nil {
		t.Fatal(err)
	}
	source[0] = 99
	restored, err := saved.restore()
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := binary.Write(&encoded, binary.LittleEndian, restored.(*MemCopyH2DCommand).Src); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded.Bytes(), []byte{11, 0, 0, 0, 13, 0, 0, 0}) {
		t.Fatal("checkpoint retained a host source reference")
	}
}
