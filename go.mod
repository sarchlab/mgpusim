module gitlab.com/akita/mgpusim

require (
	github.com/golang/mock v1.4.4
	github.com/klauspost/compress v1.11.2 // indirect
	github.com/onsi/ginkgo v1.15.0
	github.com/onsi/gomega v1.10.5
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.3.0
	github.com/vbauerster/mpb/v4 v4.12.2
	gitlab.com/akita/akita v1.11.1
	gitlab.com/akita/dnn v0.5.0
	gitlab.com/akita/mem v1.11.1
	gitlab.com/akita/noc v1.4.0
	gitlab.com/akita/util v0.7.0
)

// replace github.com/syifan/goseth => ../goseth

// replace gitlab.com/akita/akita => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem => ../mem

// replace gitlab.com/akita/util => ../util

// replace gitlab.com/akita/dnn => ../dnn

go 1.15
