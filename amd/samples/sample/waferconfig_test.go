package sample

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWaferConfigPage(t *testing.T) {
	c := Config{Timing: true, GPUType: "waferscale", GPUIDs: []int{1},
		WaferMeshWidth: 3, WaferMeshHeight: 3}
	s, _, finish := c.Build()
	defer func() { _ = finish() }()

	path := filepath.Join(t.TempDir(), "wafer.html")
	if err := writeWaferConfigPage(path, c, s); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), "<title>Wafer-Scale GPU Config</title>") ||
		!strings.Contains(string(page), `"name":"GPU[8]"`) {
		t.Fatal("the page must describe all 8 tiles of a 3x3 wafer")
	}
}

func TestWriteWaferConfigPageNeedsWaferScale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wafer.html")
	for _, c := range []Config{
		{Timing: true, GPUType: "r9nano"},
		{Timing: false, GPUType: "waferscale"},
	} {
		if err := writeWaferConfigPage(path, c, nil); err == nil {
			t.Errorf("%+v must be rejected", c)
		}
	}
}
