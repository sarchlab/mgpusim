package emugpu

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

func TestISADebugOutputClosesFileOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "isa.debug")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	output := &isaDebugOutput{ISADebugger: emu.NewISADebugger(log.New(f, "", 0)), file: f}
	for range 2 {
		if err := output.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.WriteString("after close"); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("file still open: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[\n\n]\n" {
		t.Fatalf("unexpected output %q", data)
	}
}

func TestISADebugOutputReportsClosedFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "isa.debug"))
	if err != nil {
		t.Fatal(err)
	}
	output := &isaDebugOutput{ISADebugger: emu.NewISADebugger(log.New(f, "", 0)), file: f}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if !errors.Is(output.Close(), os.ErrClosed) {
			t.Fatal("lost close or write failure")
		}
	}
}
