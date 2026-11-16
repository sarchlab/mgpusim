package training

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
)

// ExternalData identifies immutable input files. Checkpoints save this identity
// and the cursor, while normalized samples remain transient shared host data.
type ExternalData struct {
	Kind  string     `json:"Kind"`
	Files []DataFile `json:"Files"`
}
type DataFile struct {
	Path   string `json:"Path"`
	Size   int64  `json:"Size"`
	SHA256 string `json:"SHA256"`
}
type residentValues struct {
	data        []float64
	labels      []int
	fingerprint string
}

var residentCache = struct {
	sync.Mutex
	values map[string]residentValues
}{values: make(map[string]residentValues)}

func NewExternalDataSource(
	to tensor.Operator, kind string, paths []string, shape []int, descriptor string,
) (*ResidentDataSource, error) {
	state := &DataState{Shape: shape, Descriptor: descriptor, External: &ExternalData{Kind: kind}}
	for _, p := range paths {
		absolute, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(absolute)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(raw)
		state.External.Files = append(state.External.Files, DataFile{absolute, int64(len(raw)), hex.EncodeToString(sum[:])})
	}
	return BindResident(to, state)
}

// BindResident validates every external file even when an unchanged dataset is
// already cached in this process. It never advances the cursor or uses a GPU.
func BindResident(to tensor.Operator, state *DataState) (*ResidentDataSource, error) {
	d := &ResidentDataSource{State: state, Operator: to}
	if state.External == nil {
		return d, nil
	}
	values, err := loadExternal(state.External)
	if err != nil {
		return nil, err
	}
	width := 1
	for _, n := range state.Shape {
		width *= n
	}
	if width <= 0 || len(values.data) != len(values.labels)*width ||
		state.Cursor < 0 || state.Cursor > len(values.labels) {
		return nil, fmt.Errorf("dataset shape or cursor does not match external input")
	}
	d.data, d.labels = values.data, values.labels
	return d, nil
}
func loadExternal(e *ExternalData) (residentValues, error) {
	var raws [][]byte
	keys := []string{e.Kind}
	fingerprints := []string{e.Kind}
	for _, file := range e.Files {
		raw, err := os.ReadFile(file.Path)
		if err != nil {
			return residentValues{}, err
		}
		sum := sha256.Sum256(raw)
		digest := hex.EncodeToString(sum[:])
		if int64(len(raw)) != file.Size || digest != file.SHA256 {
			return residentValues{}, fmt.Errorf("dataset changed: %s", file.Path)
		}
		raws = append(raws, raw)
		keys = append(keys, file.Path)
		fingerprints = append(fingerprints, digest)
	}
	key, fingerprint := strings.Join(keys, "\x00"), strings.Join(fingerprints, "\x00")
	residentCache.Lock()
	defer residentCache.Unlock()
	if cached, ok := residentCache.values[key]; ok && cached.fingerprint == fingerprint {
		return cached, nil
	}
	var values residentValues
	var err error
	switch e.Kind {
	case "mnist":
		values, err = decodeMNIST(raws)
	case "cifar10":
		values, err = decodeCIFAR(raws)
	default:
		err = fmt.Errorf("unknown external dataset %q", e.Kind)
	}
	if err != nil {
		return residentValues{}, err
	}
	values.fingerprint = fingerprint
	residentCache.values[key] = values
	return values, nil
}
func unzip(raw []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
func decodeMNIST(files [][]byte) (residentValues, error) {
	if len(files) != 2 {
		return residentValues{}, fmt.Errorf("MNIST requires images and labels")
	}
	images, err := unzip(files[0])
	if err != nil {
		return residentValues{}, err
	}
	labels, err := unzip(files[1])
	if err != nil {
		return residentValues{}, err
	}
	if len(images) < 16 || len(labels) < 8 {
		return residentValues{}, fmt.Errorf("truncated MNIST header")
	}
	u32 := binary.BigEndian.Uint32
	n, rows, cols := uint64(u32(images[4:8])), uint64(u32(images[8:12])), uint64(u32(images[12:16]))
	if u32(images[:4]) != 2051 || u32(labels[:4]) != 2049 ||
		uint64(u32(labels[4:8])) != n || n*rows*cols != uint64(len(images)-16) || n != uint64(len(labels)-8) {
		return residentValues{}, fmt.Errorf("invalid MNIST dimensions")
	}
	v := residentValues{data: make([]float64, len(images)-16), labels: make([]int, len(labels)-8)}
	for i, x := range images[16:] {
		v.data[i] = float64(x) / 255
	}
	for i, x := range labels[8:] {
		v.labels[i] = int(x)
	}
	return v, nil
}
func decodeCIFAR(files [][]byte) (residentValues, error) {
	const record = 1 + 3*32*32
	var v residentValues
	for _, data := range files {
		if len(data)%record != 0 {
			return v, fmt.Errorf("truncated CIFAR record")
		}
		for off := 0; off < len(data); off += record {
			v.labels = append(v.labels, int(data[off]))
			for _, x := range data[off+1 : off+record] {
				v.data = append(v.data, float64(x)/255)
			}
		}
	}
	return v, nil
}
