package cifar10

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

func TestConstructorsSelectDistinctTrainingAndTestFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cifar-10-batches-bin")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	previous := *cifar10DataFolder
	*cifar10DataFolder = root
	t.Cleanup(func() { *cifar10DataFolder = previous })
	write := func(name string, label, pixel byte) {
		data := make([]byte, 3073)
		data[0] = label
		for i := 1; i < len(data); i++ {
			data[i] = pixel
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 5; i++ {
		write(fmt.Sprintf("data_batch_%d.bin", i), byte(i), byte(i*10))
	}
	write("test_batch.bin", 9, 99)
	op := tensor.CPUOperator{}
	train := NewTrainingDataSource(op)
	data, labels := train.NextBatch(5)
	if !reflect.DeepEqual(labels, []int{1, 2, 3, 4, 5}) {
		t.Fatal("training files", labels)
	}
	for image := 0; image < 5; image++ {
		for _, v := range data.Vector()[image*3072 : (image+1)*3072] {
			if v != float64((image+1)*10)/255 {
				t.Fatal("training pixel source", image, v)
			}
		}
	}
	test := NewTestDataSource(op)
	data, labels = test.NextBatch(1)
	if !reflect.DeepEqual(labels, []int{9}) {
		t.Fatal("test labels came from training data", labels)
	}
	for _, v := range data.Vector() {
		if v != float64(99)/255 {
			t.Fatal("test pixel source", v)
		}
	}
	if len(test.State.External.Files) != 1 || filepath.Base(test.State.External.Files[0].Path) != "test_batch.bin" {
		t.Fatal("test checkpoint identity", test.State.External)
	}
}
