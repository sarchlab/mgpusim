package imagenet

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

func writeFixtureImage(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestLazySourceRestoresWithoutReopeningConsumedImage(t *testing.T) {
	root := t.TempDir()
	old := *imagenetDataFolder
	*imagenetDataFolder = root
	t.Cleanup(func() { *imagenetDataFolder = old })
	dir := filepath.Join(root, "train", "class", "images")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wnids.txt"), []byte("class\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, "class_0.JPEG")
	writeFixtureImage(t, first, color.RGBA{R: 255, A: 255})
	writeFixtureImage(t, filepath.Join(dir, "class_1.JPEG"), color.RGBA{B: 255, A: 255})
	o := &tensor.CPUOperator{}
	source := NewTrainingDataSource(o)
	source.NextBatch(1)
	saved := *source.State
	if saved.Image != 1 || saved.Cursor != 1 {
		t.Fatalf("cursor %+v", saved)
	}
	expected, labels := source.NextBatch(1)
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	restored, err := Bind(o, &saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*restored.State, saved) {
		t.Fatal("restore advanced source")
	}
	actual, actualLabels := restored.NextBatch(1)
	if !reflect.DeepEqual(
		expected.Vector(),
		actual.Vector(),
	) || !reflect.DeepEqual(
		labels,
		actualLabels,
	) {
		t.Fatal("restored batch changed")
	}
	*imagenetDataFolder = t.TempDir()
	if _, err := Bind(o, &saved); err == nil {
		t.Fatal("accepted a different input root")
	}
}
