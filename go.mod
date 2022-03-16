module gitlab.com/akita/mgpusim/v3

require (
	github.com/fatih/color v1.10.0
	github.com/golang/mock v1.6.0
	github.com/gorilla/mux v1.8.0
	github.com/onsi/ginkgo/v2 v2.1.3
	github.com/onsi/gomega v1.18.1
	github.com/rs/xid v1.3.0
	github.com/tebeka/atexit v0.3.0
	gitlab.com/akita/akita/v3 v3.0.0-alpha.6
	gitlab.com/akita/dnn v0.5.3
	gitlab.com/akita/mem/v3 v3.0.0-alpha.1
	gitlab.com/akita/noc/v3 v3.0.0-alpha.1
	gonum.org/v1/gonum v0.9.0 // indirect
)

// replace github.com/syifan/goseth => ../goseth

// replace gitlab.com/akita/akita/v3 => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem/v3 => ../mem

// replace gitlab.com/akita/util/v2 => ../util

// replace gitlab.com/akita/dnn => ../dnn

go 1.16
