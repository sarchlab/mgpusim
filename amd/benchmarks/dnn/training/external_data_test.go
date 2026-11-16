package training

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

func writeDataset(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func compressedDataset(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestExternalDatasetCursorIdentityAndCompactCheckpoint(t *testing.T) {
	for _, kind := range []string{"mnist", "cifar10"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			paths, shape := externalFixture(t, kind, root)
			op := tensor.CPUOperator{}
			source, err := NewExternalDataSource(op, kind, paths, shape, "NCHW")
			if err != nil {
				t.Fatal(err)
			}
			source.NextBatch(3)
			encoded, err := json.Marshal(source.State)
			if err != nil {
				t.Fatal(err)
			}
			if len(encoded) > 1024 || len(source.State.Data) != 0 || len(source.State.Labels) != 0 {
				t.Fatal("external samples embedded", len(encoded))
			}
			var restoredState DataState
			if err := json.Unmarshal(encoded, &restoredState); err != nil {
				t.Fatal(err)
			}
			restored, err := BindResident(op, &restoredState)
			if err != nil {
				t.Fatal(err)
			}
			if &source.data[0] != &restored.data[0] {
				t.Fatal("unchanged external dataset was duplicated")
			}
			want, labels := source.NextBatch(2)
			got, gotLabels := restored.NextBatch(2)
			if !reflect.DeepEqual(want.Vector(), got.Vector()) || !reflect.DeepEqual(labels, gotLabels) ||
				restoredState.Cursor != 5 {
				t.Fatal("cursor or values changed on bind")
			}
			checkFreshExternal(t, kind, root, encoded, externalExpected{Values: want.Vector(), Labels: labels, Cursor: 5})
			checkExternalMutation(t, op, &restoredState, paths[0])
		})
	}
}

type externalExpected struct {
	Values []float64 `json:"values"`
	Labels []int     `json:"labels"`
	Cursor int       `json:"cursor"`
}

func TestExternalDatasetFreshProcessReload(t *testing.T) {
	path := os.Getenv("MGPUSIM_EXTERNAL_DATA_RESTORE")
	if path == "" {
		return
	} // Invoked in fresh children by the fixture test above.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state DataState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path + ".expected")
	if err != nil {
		t.Fatal(err)
	}
	var expected externalExpected
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if len(residentCache.values) != 0 {
		t.Fatal("child cache was not empty")
	}
	source, err := BindResident(tensor.CPUOperator{}, &state)
	if err != nil {
		t.Fatal(err)
	}
	got, labels := source.NextBatch(2)
	if !reflect.DeepEqual(got.Vector(), expected.Values) || !reflect.DeepEqual(labels, expected.Labels) ||
		state.Cursor != expected.Cursor {
		t.Fatal("fresh reload did not resume identical data at cursor")
	}
	t.Logf("%s cursor=%d, %d normalized values match", state.External.Kind, state.Cursor, len(expected.Values))
}

func externalFixture(t *testing.T, kind, root string) ([]string, []int) {
	t.Helper()

	var paths []string
	var shape []int
	if kind == "mnist" {
		header := []uint32{2051, 20, 28, 28}
		var b bytes.Buffer
		if err := binary.Write(&b, binary.BigEndian, header); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20*28*28; i++ {
			b.WriteByte(byte(i))
		}
		images := compressedDataset(t, b.Bytes())
		b.Reset()
		if err := binary.Write(&b, binary.BigEndian, []uint32{2049, 20}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			b.WriteByte(byte(i % 10))
		}
		labels := compressedDataset(t, b.Bytes())
		paths = []string{filepath.Join(root, "images.gz"), filepath.Join(root, "labels.gz")}
		writeDataset(t, paths[0], images)
		writeDataset(t, paths[1], labels)
		shape = []int{1, 28, 28}
	} else {
		raw := make([]byte, 20*3073)
		for i := range raw {
			raw[i] = byte(i)
		}
		paths = []string{filepath.Join(root, "batch.bin")}
		writeDataset(t, paths[0], raw)
		shape = []int{3, 32, 32}
	}

	return paths, shape
}

func checkFreshExternal(t *testing.T, kind, root string, encoded []byte, want externalExpected) {
	t.Helper()
	// Launch a fresh process with an empty resident cache.
	statePath := filepath.Join(root, "cursor.json")
	writeDataset(t, statePath, encoded)
	expected, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	writeDataset(t, statePath+".expected", expected)
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExternalDatasetFreshProcessReload$", "-test.v")
	cmd.Env = append(os.Environ(), "MGPUSIM_EXTERNAL_DATA_RESTORE="+statePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh %s: %v\n%s", kind, err, out)
	} else {
		t.Logf("fresh %s reload: %s", kind, out)
	}
}

func checkExternalMutation(t *testing.T, op tensor.Operator, state *DataState, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), raw...)
	changed[len(changed)-1] ^= 1
	writeDataset(t, path, changed)
	if _, err := BindResident(op, state); err == nil {
		t.Fatal("same-process cache accepted changed file")
	}
	writeDataset(t, path, raw)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := BindResident(op, state); err == nil {
		t.Fatal("missing file accepted")
	}
}
