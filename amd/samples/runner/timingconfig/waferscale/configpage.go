package waferscale

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/sarchlab/akita/v5/sim"
)

// ConfigPage is what the configuration page of a wafer-scale GPU shows: the
// wafer on the first level and the components of each GPU on the second.
type ConfigPage struct {
	Mesh          Mesh   `json:"mesh"`
	TileType      string `json:"tileType"`
	Tile          Tile   `json:"tile"`
	HostLocation  [3]int `json:"hostLocation"`
	SwitchLatency int    `json:"switchLatency"`
	FlitSize      int    `json:"flitSize"`

	// Host holds the components on the host position: the driver, the
	// IOMMU, and the CPU memory.
	Host []ComponentGroup `json:"host"`

	// Network holds the switches and endpoints of the mesh.
	Network []ComponentGroup `json:"network"`

	GPUs []GPUConfig `json:"gpus"`
}

// GPUConfig is one GPU tile and its components.
type GPUConfig struct {
	Index    int              `json:"index"`
	Name     string           `json:"name"`
	Location [3]int           `json:"location"`
	Groups   []ComponentGroup `json:"groups"`
}

// ComponentGroup holds the components of one kind, such as all the CUs of a
// GPU. Kind is the name inside its device with the indices removed, such as
// "SA[].CU[]". The spec of each component is flattened into leaf fields, in
// the order of the spec struct. Most fields are the same for every component
// of the kind; the rest, usually the ports each component sends to, are
// listed per component.
type ComponentGroup struct {
	Kind  string   `json:"kind"`
	Type  string   `json:"type"`
	Names []string `json:"names"`

	// Common holds the fields every component of the kind shares.
	Common []Field `json:"common"`

	// Differing names the fields that are not the same for all the
	// components; Values[i][j] is field j of component Names[i], or empty
	// when the component has no such field.
	Differing []string   `json:"differing"`
	Values    [][]string `json:"values"`
}

// Field is one leaf of a spec: its path, such as "bottom.destinations[0]",
// and its value as JSON.
type Field struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

var indexPattern = regexp.MustCompile(`\[\d+\]`)

//go:embed configpage.html.tmpl
var configPageHTML string

var configPageTemplate = template.Must(template.New("configpage").Parse(configPageHTML))

// WriteConfigPage writes the configuration page as one self-contained HTML
// file: the wafer on the first level and, when a tile is clicked, the
// components of its GPU on the second.
func WriteConfigPage(w io.Writer, page ConfigPage) error {
	return configPageTemplate.Execute(w, page)
}

// WriteConfigPageFile collects the configuration page of a built wafer-scale
// GPU of the given GPU type and mesh, and writes it to the file at path.
func WriteConfigPageFile(path string, m Mesh, gpuType string, components []sim.Component) error {
	tile, ok := ParseGPUType(gpuType)
	if !ok {
		return fmt.Errorf("waferscale: %q is not a wafer-scale GPU type", gpuType)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	if err := WriteConfigPage(f, CollectConfigPage(m, tile, components)); err != nil {
		_ = f.Close()
		return err
	}

	return f.Close()
}

// CollectConfigPage reads the specs of the components of a built wafer-scale
// platform. GPU i is the component name prefix "GPU[i]."; the mesh is
// "Mesh."; every other component is on the host.
func CollectConfigPage(m Mesh, tileType string, components []sim.Component) ConfigPage {
	page := ConfigPage{
		Mesh:          m,
		TileType:      tileType,
		Tile:          TileByName(tileType),
		HostLocation:  m.HostLocation(),
		SwitchLatency: SwitchLatency,
		FlitSize:      FlitSize,
	}

	host, network := newGrouper(), newGrouper()
	gpus := make([]*grouper, m.NumTiles())
	for i := range gpus {
		gpus[i] = newGrouper()
	}

	for _, c := range components {
		name := c.Name()
		switch {
		case strings.HasPrefix(name, "Mesh."):
			network.add(c, strings.TrimPrefix(name, "Mesh."))
		case strings.HasPrefix(name, "GPU["):
			index, local := splitGPUName(name)
			if index < 1 || index > len(gpus) {
				panic(fmt.Sprintf("waferscale: component %s is on no tile", name))
			}
			gpus[index-1].add(c, local)
		default:
			host.add(c, name)
		}
	}

	page.Host = host.groups()
	page.Network = network.groups()
	for i, g := range gpus {
		page.GPUs = append(page.GPUs, GPUConfig{
			Index:    i + 1,
			Name:     fmt.Sprintf("GPU[%d]", i+1),
			Location: m.TileLocation(i + 1),
			Groups:   g.groups(),
		})
	}

	return page
}

// splitGPUName splits "GPU[3].SA[0].CU[1]" into 3 and "SA[0].CU[1]".
func splitGPUName(name string) (index int, local string) {
	device, local, _ := strings.Cut(name, ".")
	if _, err := fmt.Sscanf(device, "GPU[%d]", &index); err != nil {
		panic(fmt.Sprintf("waferscale: cannot read the GPU of component %s", name))
	}

	return index, local
}

// grouper collects components into groups, in the order each kind first
// appears, which is the order the builders create them in.
type grouper struct {
	order []string
	kinds map[string]*pendingGroup
}

// pendingGroup is a group whose fields are not yet split into common and
// differing ones.
type pendingGroup struct {
	typ    string
	names  []string
	fields [][]Field
}

func newGrouper() *grouper {
	return &grouper{kinds: make(map[string]*pendingGroup)}
}

func (g *grouper) add(c sim.Component, local string) {
	fields, typ := specFields(c)
	kind := indexPattern.ReplaceAllString(local, "[]")

	group := g.kinds[kind]
	if group == nil {
		group = &pendingGroup{typ: typ}
		g.kinds[kind] = group
		g.order = append(g.order, kind)
	}
	group.names = append(group.names, local)
	group.fields = append(group.fields, fields)
}

func (g *grouper) groups() []ComponentGroup {
	groups := make([]ComponentGroup, 0, len(g.order))
	for _, kind := range g.order {
		groups = append(groups, g.kinds[kind].split(kind))
	}

	return groups
}

// split divides the fields into the ones all the components share and the
// ones that differ, keeping the order in which the fields first appear.
func (p *pendingGroup) split(kind string) ComponentGroup {
	var paths []string
	values := make([]map[string]string, len(p.fields))
	for i, fields := range p.fields {
		values[i] = make(map[string]string, len(fields))
		for _, f := range fields {
			paths = append(paths, f.Path)
			values[i][f.Path] = f.Value
		}
	}

	group := ComponentGroup{Kind: kind, Type: p.typ, Names: p.names}
	for _, path := range uniqueInOrder(paths) {
		if value, same := sameValue(values, path); same {
			group.Common = append(group.Common, Field{Path: path, Value: value})
		} else {
			group.Differing = append(group.Differing, path)
		}
	}

	for i := range p.names {
		row := make([]string, len(group.Differing))
		for j, path := range group.Differing {
			row[j] = values[i][path]
		}
		group.Values = append(group.Values, row)
	}

	return group
}

func sameValue(values []map[string]string, path string) (string, bool) {
	first, ok := values[0][path]
	if !ok {
		return "", false
	}
	for _, v := range values[1:] {
		if value, ok := v[path]; !ok || value != first {
			return "", false
		}
	}

	return first, true
}

func uniqueInOrder(items []string) []string {
	seen := make(map[string]bool, len(items))
	unique := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			unique = append(unique, item)
		}
	}

	return unique
}

// specFields returns the leaf fields of the component's exported Spec field
// and the field's Go type, the way Akita's topology recorder reads the spec.
// A component without a spec, or whose spec does not serialize, has no
// fields.
func specFields(c sim.Component) ([]Field, string) {
	v := reflect.Indirect(reflect.ValueOf(c))
	if v.Kind() != reflect.Struct {
		return nil, reflect.TypeOf(c).String()
	}

	f := v.FieldByName("Spec")
	if !f.IsValid() || !f.CanInterface() || f.Interface() == nil {
		return nil, reflect.TypeOf(c).String()
	}

	data, err := json.Marshal(f.Interface())
	if err != nil {
		return nil, f.Type().String()
	}

	fields, err := flattenJSON(data)
	if err != nil {
		return nil, f.Type().String()
	}

	return fields, f.Type().String()
}

// flattenJSON lists the leaves of a JSON document in document order, which
// for a marshalled struct is the order of its fields. Object keys join with
// dots and array elements take [i]; an empty object or array is a leaf.
func flattenJSON(data []byte) ([]Field, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var fields []Field
	err := flattenTokens(dec, "", &fields)

	return fields, err
}

func flattenTokens(dec *json.Decoder, path string, fields *[]Field) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}

	switch tok {
	case json.Delim('{'):
		empty := true
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			if err := flattenTokens(dec, joinPath(path, key.(string)), fields); err != nil {
				return err
			}
			empty = false
		}
		if empty {
			*fields = append(*fields, Field{Path: path, Value: "{}"})
		}
		_, err = dec.Token() // the closing brace

		return err
	case json.Delim('['):
		i := 0
		for ; dec.More(); i++ {
			if err := flattenTokens(dec, fmt.Sprintf("%s[%d]", path, i), fields); err != nil {
				return err
			}
		}
		if i == 0 {
			*fields = append(*fields, Field{Path: path, Value: "[]"})
		}
		_, err = dec.Token() // the closing bracket

		return err
	default:
		value, err := json.Marshal(tok)
		if err != nil {
			return err
		}
		*fields = append(*fields, Field{Path: path, Value: string(value)})

		return nil
	}
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}

	return path + "." + key
}
