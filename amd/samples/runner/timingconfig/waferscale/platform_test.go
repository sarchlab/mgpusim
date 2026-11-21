// The platform tests build a whole wafer through timingconfig. They are an
// external test package because timingconfig imports waferscale.
package waferscale_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm/pagetablemmu"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

// buildWaferScale initializes a wafer-scale platform with the given mesh and
// returns its simulation.
func buildWaferScale(t *testing.T, m waferscale.Mesh) *simulation.Simulation {
	t.Helper()

	s := simulation.MakeBuilder().
		WithOutputPath(t.TempDir() + "/sim.sqlite3").
		Build()
	t.Cleanup(func() { _ = s.Terminate() })

	r := timingconfig.MakeBuilder().WithSimulation(s).WithGPUType("waferscale").
		WithWaferScaleMesh(m.Width, m.Height).Build()
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	if r.GetNumGPUs() != m.NumTiles() {
		t.Fatalf("expected %d GPUs, got %d", m.NumTiles(), r.GetNumGPUs())
	}

	return s
}

func countMeshSwitches(s *simulation.Simulation) int {
	n := 0
	for _, c := range s.Components() {
		if strings.HasPrefix(c.Name(), "Mesh.SW[") {
			n++
		}
	}

	return n
}

func findGroup(groups []waferscale.ComponentGroup, kind string) *waferscale.ComponentGroup {
	for i := range groups {
		if groups[i].Kind == kind {
			return &groups[i]
		}
	}

	return nil
}

// TestWaferScaleMeshSize checks a 7x7 wafer, the size on Akkalat's v4
// branch: 48 tiles and one switch per position.
func TestWaferScaleMeshSize(t *testing.T) {
	s := buildWaferScale(t, waferscale.Mesh{Width: 7, Height: 7})

	if n := countMeshSwitches(s); n != 49 {
		t.Fatalf("expected 49 mesh switches, got %d", n)
	}
}

// TestWaferScaleUsesMesh checks that the host and the tiles share a mesh
// with one switch per position. The direct connection has no switches.
func TestWaferScaleUsesMesh(t *testing.T) {
	s := buildWaferScale(t, waferscale.DefaultMesh)

	want := waferscale.DefaultMesh.Width * waferscale.DefaultMesh.Height
	if numSwitches := countMeshSwitches(s); numSwitches != want {
		t.Fatalf("expected %d mesh switches, got %d", want, numSwitches)
	}
}

// TestWaferScaleIOMMUPool checks that the IOMMU pool can map the memory of
// all the tiles.
func TestWaferScaleIOMMUPool(t *testing.T) {
	s := buildWaferScale(t, waferscale.DefaultMesh)

	for _, c := range s.Components() {
		if io, ok := c.(*pagetablemmu.Comp); ok && io.Name() == "IOMMU" {
			// 256 MiB for 24 x 4 GiB tiles with 4 KiB pages (sarchlab review).
			if io.Spec.PoolNodes != 65_536 {
				t.Fatalf("expected 65536 IOMMU pool nodes, got %d", io.Spec.PoolNodes)
			}
			return
		}
	}

	t.Fatal("IOMMU must be registered with the simulation")
}

// TestWaferScaleConfigPage checks that the configuration page places every
// component of a built wafer on the host, the mesh, or its GPU tile.
func TestWaferScaleConfigPage(t *testing.T) {
	m := waferscale.DefaultMesh
	s := buildWaferScale(t, m)

	page := waferscale.CollectConfigPage(m, waferscale.DefaultTile, s.Components())

	if len(page.GPUs) != m.NumTiles() {
		t.Fatalf("expected %d GPUs, got %d", m.NumTiles(), len(page.GPUs))
	}
	for _, name := range []string{"Driver", "IOMMU", "CPU.Memory"} {
		if findGroup(page.Host, name) == nil {
			t.Errorf("host must have %s", name)
		}
	}
	if g := findGroup(page.Network, "SW[][][]"); g == nil || len(g.Names) != m.Width*m.Height {
		t.Errorf("network must have %d switches", m.Width*m.Height)
	}

	gpu := page.GPUs[12] // GPU[13], the first tile after the host
	if gpu.Name != "GPU[13]" || gpu.Location != [3]int{3, 2, 0} {
		t.Fatalf("unexpected tile %s at %v", gpu.Name, gpu.Location)
	}

	// The CUs of a tile share their configuration and differ only in the
	// ports inside the tile that each one sends to.
	tile := waferscale.R9NanoTile
	cus := findGroup(gpu.Groups, "SA[].CU[]")
	if cus == nil || len(cus.Names) != tile.NumCUPerShaderArray*tile.NumShaderArray {
		t.Fatalf("expected %d CUs", tile.NumCUPerShaderArray*tile.NumShaderArray)
	}
	if len(cus.Common) == 0 || len(cus.Differing) == 0 || len(cus.Values) != len(cus.Names) {
		t.Fatalf("expected common and per-CU fields, got %d and %v", len(cus.Common), cus.Differing)
	}
	for i, row := range cus.Values {
		for j, value := range row {
			if !strings.HasPrefix(value, `"GPU[13].`) {
				t.Errorf("%s field %s = %s is not a port of the tile",
					cus.Names[i], cus.Differing[j], value)
			}
		}
	}

	var html strings.Builder
	if err := waferscale.WriteConfigPage(&html, page); err != nil {
		t.Fatal(err)
	}

	// The page embeds its data as a JSON value in the script.
	_, data, found := strings.Cut(html.String(), "const PAGE = ")
	data, _, _ = strings.Cut(data, ";\n")
	var decoded waferscale.ConfigPage
	if !found || json.Unmarshal([]byte(data), &decoded) != nil {
		t.Fatal("the page must embed its data as JSON")
	}
	if len(decoded.GPUs) != m.NumTiles() || decoded.GPUs[12].Name != "GPU[13]" {
		t.Fatal("the embedded data must describe every tile")
	}
}

// TestWriteConfigPageFile writes the page of a 3x3 wafer to a file and
// rejects a GPU type that is not wafer-scale.
func TestWriteConfigPageFile(t *testing.T) {
	m := waferscale.Mesh{Width: 3, Height: 3}
	s := buildWaferScale(t, m)
	path := filepath.Join(t.TempDir(), "wafer.html")

	if err := waferscale.WriteConfigPageFile(path, m, "waferscale", s.Components()); err != nil {
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

	if err := waferscale.WriteConfigPageFile(path, m, "r9nano", s.Components()); err == nil {
		t.Fatal("an r9nano GPU must be rejected")
	}
}
