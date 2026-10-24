package runner

import (
	"database/sql"
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplicitMetricFileReplacesPreviousRun(t *testing.T) {
	oldFlags, oldFilename, oldDisableRTM := flag.CommandLine, filenameFlag, disableAkitaRTM
	t.Cleanup(func() {
		flag.CommandLine, filenameFlag, disableAkitaRTM = oldFlags, oldFilename, oldDisableRTM
	})

	for _, name := range []string{"results", "results.sqlite3"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			flag.CommandLine = flag.NewFlagSet("runner", flag.ContinueOnError)
			filenameFlag = flag.CommandLine.String("metric-file-name", "metrics", "")
			disableAkitaRTM = flag.CommandLine.Bool("disable-rtm", true, "")
			if err := flag.CommandLine.Parse([]string{"-metric-file-name", path}); err != nil {
				t.Fatal(err)
			}

			first := new(Runner)
			first.initSimulation()
			first.simulation.DataRecorder().CreateTable("previous_run", struct{ Value int }{})
			if err := first.simulation.DataRecorder().Close(); err != nil {
				t.Fatal(err)
			}

			second := new(Runner)
			second.initSimulation()
			t.Cleanup(func() { _ = second.simulation.DataRecorder().Close() })
			if !strings.HasSuffix(path, ".sqlite3") {
				path += ".sqlite3"
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var tables int
			err = db.QueryRowContext(t.Context(),
				"SELECT count(*) FROM sqlite_master WHERE name = 'previous_run'").Scan(&tables)
			if err != nil {
				t.Fatal(err)
			}
			if tables != 0 {
				t.Fatal("the second run retained the previous recording")
			}
		})
	}
}
