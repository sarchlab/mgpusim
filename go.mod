module gitlab.com/akita/mgpusim

require (
	github.com/go-sql-driver/mysql v1.5.0 // indirect
	github.com/golang/mock v1.4.3
	github.com/klauspost/compress v1.10.8 // indirect
	github.com/onsi/ginkgo v1.13.0
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rs/xid v1.2.1
	github.com/tebeka/atexit v0.3.0
	github.com/vbauerster/mpb/v4 v4.12.2
	gitlab.com/akita/akita v1.10.1
	gitlab.com/akita/dnn v0.3.0
	gitlab.com/akita/mem v1.8.6
	gitlab.com/akita/noc v1.4.0
	gitlab.com/akita/util v0.6.3
	golang.org/x/crypto v0.0.0-20200604202706-70a84ac30bf9 // indirect
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a // indirect
)

// replace gitlab.com/akita/akita => ../akita

// replace gitlab.com/akita/noc => ../noc

// replace gitlab.com/akita/mem => ../mem

// replace gitlab.com/akita/util => ../util

// replace gitlab.com/akita/dnn => ../dnn

go 1.13
