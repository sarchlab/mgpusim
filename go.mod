module github.com/sarchlab/mgpusim/v4

require (
	github.com/disintegration/imaging v1.6.2
	github.com/fatih/color v1.18.0
	github.com/golang/mock v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/onsi/ginkgo/v2 v2.22.2
	github.com/onsi/gomega v1.36.2
	github.com/rs/xid v1.6.0
	github.com/sarchlab/akita/v4 v4.0.0 // v3.0.0
	github.com/tebeka/atexit v0.3.0
	gonum.org/v1/gonum v0.15.1
)

require github.com/sirupsen/logrus v1.9.3

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-sql-driver/mysql v1.9.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20250208200701-d0013a598941 // indirect
	github.com/kr/pretty v0.2.1 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.24 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/syifan/goseth v0.1.2 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.9.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/image v0.24.0 // indirect
	golang.org/x/net v0.37.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/syifan/goseth => ../goseth

// replace github.com/sarchlab/akita/v4 => ../akita

go 1.23.0

toolchain go1.23.3
