package timingconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

func findGroup(groups []waferscale.ComponentGroup, kind string) *waferscale.ComponentGroup {
	for i := range groups {
		if groups[i].Kind == kind {
			return &groups[i]
		}
	}

	return nil
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
