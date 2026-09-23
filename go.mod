module github.com/sarchlab/mgpusim/v5

require (
	github.com/disintegration/imaging v1.6.2
	github.com/fatih/color v1.19.0
	github.com/gorilla/mux v1.8.1
	github.com/onsi/ginkgo/v2 v2.32.1
	github.com/onsi/gomega v1.43.0
	github.com/sarchlab/akita/v5 v5.0.0-beta.10
	github.com/tebeka/atexit v0.3.0
	go.uber.org/mock v0.6.0
	gonum.org/v1/gonum v0.17.0
)

require (
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/glebarez/go-sqlite v1.23.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20260825171938-4d453200e7d9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-sqlite3 v1.14.50 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/syifan/goseth v0.1.2 // indirect
	github.com/tklauser/go-sysconf v0.4.0 // indirect
	github.com/tklauser/numcpus v0.12.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/image v0.45.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	modernc.org/libc v1.75.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.57.0 // indirect
)

// replace github.com/syifan/goseth => ../goseth

// replace github.com/sarchlab/akita/v5 => ../../../../akita

go 1.27.0

// Retained dependency-security guard: tebeka/atexit still reaches testify
// v1.5.1's stale yaml.v2 requirement through its dependency graph. Remove this
// guard once `go list -m all` no longer selects yaml.v2 v2.2.2 without it.
exclude gopkg.in/yaml.v2 v2.2.2
