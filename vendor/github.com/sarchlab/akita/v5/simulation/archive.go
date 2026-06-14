package simulation

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"time"
)

// A checkpoint archive is a tar.gz holding a build-identity entry and one
// payload per registered entity; the payload files are the inventory, so there
// is no manifest. Each entity owns the bytes of its own payload.
const (
	buildIDPath  = "build_id"
	entityPrefix = "entities/"
)

// Checkpointable is implemented by runtime entities that save and load their own
// payload bytes. The entity owns its encoding (JSON, binary, ...); the archive
// only shuttles bytes. It is satisfied structurally, so an entity package never
// imports the simulation package to be checkpointable.
type Checkpointable interface {
	SaveCheckpoint(w io.Writer) error
	LoadCheckpoint(r io.Reader) error
}

// archiveEntry is one entity payload to write into a checkpoint archive.
type archiveEntry struct {
	name string
	data []byte
}

// entityPath returns the archive path for an entity's payload. The name is
// escaped so it can never become a real filesystem path.
func entityPath(name string) string {
	return entityPrefix + url.PathEscape(name)
}

// entityName reverses entityPath, returning false for non-entity archive paths.
func entityName(archivePath string) (string, bool) {
	if !strings.HasPrefix(archivePath, entityPrefix) {
		return "", false
	}

	name, err := url.PathUnescape(strings.TrimPrefix(archivePath, entityPrefix))
	if err != nil {
		return "", false
	}
	return name, true
}

// defaultBuildID returns a deterministic fingerprint for the current executable.
// It is intentionally process-local: checkpoints are expected to be restored by
// the same binary that produced them.
func defaultBuildID() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return hashStrings("unknown-build-info")
	}

	parts := []string{
		"path=" + info.Path,
		"main=" + info.Main.Path + "@" + info.Main.Version,
	}

	for _, dep := range info.Deps {
		parts = append(parts, "dep="+dep.Path+"@"+dep.Version)
		if dep.Replace != nil {
			parts = append(parts,
				"replace="+dep.Replace.Path+"@"+dep.Replace.Version)
		}
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs", "vcs.revision", "vcs.modified", "vcs.time":
			parts = append(parts, setting.Key+"="+setting.Value)
		}
	}

	sort.Strings(parts)
	return hashStrings(parts...)
}

func hashStrings(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = io.WriteString(h, part)
		_, _ = io.WriteString(h, "\n")
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeArchive atomically writes a checkpoint archive: the build identity
// followed by entity payloads sorted by name.
func writeArchive(path, buildID string, entries []archiveEntry) error {
	if buildID == "" {
		return errors.New("checkpoint: build ID is required")
	}

	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	err = writeArchiveStream(file, buildID, entries)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	cleanup = false
	return nil
}

func writeArchiveStream(w io.Writer, buildID string, entries []archiveEntry) error {
	gz := gzip.NewWriter(w)
	gz.ModTime = time.Unix(0, 0)

	tw := tar.NewWriter(gz)

	if err := writeTarEntry(tw, buildIDPath, []byte(buildID)); err != nil {
		return err
	}

	sorted := append([]archiveEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].name < sorted[j].name
	})

	seen := map[string]struct{}{}
	for _, entry := range sorted {
		if _, dup := seen[entry.name]; dup {
			return fmt.Errorf("checkpoint: duplicate entity %q", entry.name)
		}
		seen[entry.name] = struct{}{}

		if err := writeTarEntry(tw, entityPath(entry.name), entry.data); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeTarEntry(tw *tar.Writer, path string, data []byte) error {
	header := &tar.Header{
		Name:    path,
		Mode:    0o600,
		Size:    int64(len(data)),
		ModTime: time.Unix(0, 0),
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err := tw.Write(data)
	return err
}

// readArchive reads a checkpoint archive, returning the build identity and the
// entity payloads keyed by entity name. Unknown archive entries are rejected.
func readArchive(path string) (string, map[string][]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	return readArchiveStream(file)
}

func readArchiveStream(r io.Reader) (string, map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	payloads := make(map[string][]byte)
	buildID := ""
	foundBuildID := false

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != 0 {
			return "", nil,
				fmt.Errorf("checkpoint: unsupported archive entry %q", header.Name)
		}

		data, err := io.ReadAll(tr)
		if err != nil {
			return "", nil, err
		}

		if header.Name == buildIDPath {
			if foundBuildID {
				return "", nil, fmt.Errorf("checkpoint: duplicate %s", buildIDPath)
			}
			buildID = string(data)
			foundBuildID = true
			continue
		}

		name, ok := entityName(header.Name)
		if !ok {
			return "", nil,
				fmt.Errorf("checkpoint: unexpected archive entry %q", header.Name)
		}
		if _, dup := payloads[name]; dup {
			return "", nil, fmt.Errorf("checkpoint: duplicate entity %q", name)
		}
		payloads[name] = data
	}

	if !foundBuildID {
		return "", nil, fmt.Errorf("checkpoint: missing %s", buildIDPath)
	}
	if buildID == "" {
		return "", nil, errors.New("checkpoint: build ID is empty")
	}

	return buildID, payloads, nil
}
