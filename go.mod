module gitlab.com/akita/mgpusim

require (
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/golang/mock v1.4.4
	github.com/klauspost/compress v1.10.11 // indirect
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.3.0
	github.com/vbauerster/mpb/v4 v4.12.2
	gitlab.com/akita/akita v1.10.1
	gitlab.com/akita/dnn v0.3.0
	gitlab.com/akita/mem v1.8.7
	gitlab.com/akita/noc v1.4.0
	gitlab.com/akita/util v0.6.4
	golang.org/x/crypto v0.0.0-20200820211705-5c72a883971a // indirect
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208 // indirect
)

// replace gitlab.com/akita/akita => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem => ../mem

// replace gitlab.com/akita/util => ../util

// replace gitlab.com/akita/dnn => ../dnn

go 1.13
