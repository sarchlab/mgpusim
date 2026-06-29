package runner

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sarchlab/akita/v5/simulation"
)

// mgpusimModulePath is mgpusim's Go module path. The source archive recorded into
// a vis trace is keyed by it (mirrors how Akita keys its own source).
const mgpusimModulePath = "github.com/sarchlab/mgpusim/v5"

// withRecordedSource registers mgpusim's own source tree so it is archived into
// the vis trace alongside Akita's, making the trace self-describing (e.g. for
// DaisenBot). It reads from disk like Akita does for its own source; the recorder
// keeps only non-test .go plus go.mod and prunes VCS/vendor/.claude dirs, so no
// kernel binaries or fixtures are recorded. When the source is not on disk (e.g.
// a -trimpath build), nothing is registered — matching Akita's behavior.
func withRecordedSource(b simulation.Builder) simulation.Builder {
	dir, ok := mgpusimSourceDir()
	if !ok {
		return b
	}

	return b.WithSourceFS(mgpusimModulePath, os.DirFS(dir))
}

// mgpusimSourceDir returns the on-disk root of the mgpusim module and true if
// found. It locates itself from the compiled-in path of this file and walks up to
// the enclosing go.mod that declares mgpusimModulePath, so it resolves whether
// mgpusim is the main module, a module-cache dependency, or a replace target.
func mgpusimSourceDir() (string, bool) {
	_, file, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(file) {
		return "", false
	}

	dir := filepath.Dir(file)
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && moduleLineIs(data, mgpusimModulePath) {
			if _, statErr := os.Stat(dir); statErr != nil {
				return "", false
			}
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// moduleLineIs reports whether a go.mod's `module` line declares modulePath.
func moduleLineIs(gomod []byte, modulePath string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(gomod))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "module" {
			return fields[1] == modulePath
		}
	}

	return false
}
