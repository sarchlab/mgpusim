package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"

	"gitlab.com/yaotsu/gcn3/trace/instpb"

	"encoding/json"

	"github.com/golang/protobuf/proto"
)

const usageMessage = "" +
	`
Usage of vis 
	vis [flags] trace.out

Flags
	-http=addr: HTTP service address (e.g., ':6060')
	`

var (
	httpFlag  = flag.String("http", "localhost:0", "HTTP service address (e.g., ':6060')")
	traceFile string
)

func main() {
	parseArgs()
	parseTrace()
	startServer()
}

func parseArgs() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, usageMessage)
		os.Exit(2)
	}

	flag.Parse()

	switch flag.NArg() {
	case 1:
		traceFile = flag.Arg(0)
	default:
		flag.Usage()
	}
}

var trace = make([]*instpb.Inst, 0)

func parseTrace() {
	f, err := os.Open(traceFile)
	dieOnErr(err)

	var length uint32
	for {
		err = binary.Read(f, binary.LittleEndian, &length)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Panic(err)
		}

		buf := make([]byte, length)
		n, err := f.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Panic(err)
		}
		if uint32(n) != length {
			log.Panic(errors.New("No enough bytes to load"))
		}

		instTraceItem := new(instpb.Inst)
		err = proto.Unmarshal(buf, instTraceItem)
		dieOnErr(err)

		trace = append(trace, instTraceItem)
	}

	log.Printf("%d", len(trace))
	sort.Slice(trace, func(i, j int) bool {
		return trace[i].Events[0].Time < trace[j].Events[0].Time
	})
}

func startServer() {
	ln, err := net.Listen("tcp", *httpFlag)
	dieOnErr(err)

	openbrowser("http://" + ln.Addr().String())

	http.HandleFunc("/trace", httpTrace)
	http.HandleFunc("/minimap", httpMinimap)
	http.Handle("/", http.FileServer(http.Dir("www")))
	err = http.Serve(ln, nil)
	dieOnErr(err)
}

func httpTrace(w http.ResponseWriter, r *http.Request) {
	start, err := strconv.Atoi(r.FormValue("start"))
	dieOnErr(err)

	end, err := strconv.Atoi(r.FormValue("end"))
	dieOnErr(err)

	respond := "["
	for i := start; i < end; i++ {
		bytes, err := json.Marshal(trace[i])
		dieOnErr(err)

		if i != start {
			respond += ","
		}
		respond += string(bytes)
	}
	respond += "]"

	_, err = w.Write([]byte(respond))
	dieOnErr(err)
}

func httpMinimap(w http.ResponseWriter, r *http.Request) {

}

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}

	dieOnErr(err)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
