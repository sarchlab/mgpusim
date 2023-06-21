module github.com/sarchlab/mgpusim/v3

require (
	github.com/fatih/color v1.15.0
	github.com/golang/mock v1.6.0
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo/v2 v2.9.7
	github.com/onsi/gomega v1.27.7
	github.com/rs/xid v1.5.0
	github.com/sarchlab/akita/v3 v3.0.0-alpha.28.0.20230616154900-5e2fda40a106
	github.com/tebeka/atexit v0.3.0
	gitlab.com/akita/dnn v0.5.4
)

require (
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-sql-driver/mysql v1.7.1 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/pprof v0.0.0-20230510103437-eeec1cb781c3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/shirou/gopsutil v3.21.11+incompatible // indirect
	github.com/stretchr/testify v1.7.0 // indirect
	github.com/syifan/goseth v0.1.1 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	golang.org/x/image v0.7.0 // indirect
	golang.org/x/net v0.10.0 // indirect
	golang.org/x/sys v0.9.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/tools v0.9.1 // indirect
	gonum.org/v1/gonum v0.13.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/syifan/goseth => ../goseth

replace github.com/sarchlab/akita/v3 => ../akita

// replace github.com/sarchlab/mgpusim/v3/noc/ => ../noc

// replace github.com/sarchlab/akita/v3/mem/ => ../mem

// replace gitlab.com/akita/dnn => ../dnn

go 1.19
