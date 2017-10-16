package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	hod "github.com/gtfierro/hod/clients/go"
	"github.com/op/go-logging"
	"goji.io"
	"goji.io/pat"
	bw2 "gopkg.in/immesys/bw2bind.v5"
)

// logger
var log *logging.Logger

// set up logging facilities
func init() {
	log = logging.MustGetLogger("sWAP")
	var format = "%{color}%{level} %{time:Jan 02 15:04:05} %{shortfile}%{color:reset} ▶ %{message}"
	var logBackend = logging.NewLogBackend(os.Stderr, "", 0)
	logBackendLeveled := logging.AddModuleLevel(logBackend)
	logging.SetBackend(logBackendLeveled)
	logging.SetFormatter(logging.MustStringFormatter(format))
}

type server struct {
	mux          *goji.Mux
	hod          *hod.HodClientBW2
	bw2          *bw2.BW2Client
	num_received uint64
	num_metadata uint64
	num_readings uint64
}

func startServer(address string, hoduri string) {

	s := &server{
		mux:          goji.NewMux(),
		num_received: 0,
		num_metadata: 0,
		num_readings: 0,
	}

	go func() {
		tick := time.NewTicker(10 * time.Second)
		for _ = range tick.C {
			received := atomic.SwapUint64(&s.num_received, 0)
			metadata := atomic.SwapUint64(&s.num_metadata, 0)
			readings := atomic.SwapUint64(&s.num_readings, 0)
			fmt.Printf("%s: msgs/metadata/timeseries = %d/%d/%d\n", time.Now(), received, metadata, readings)
		}
	}()

	// define Hod client
	s.bw2 = bw2.ConnectOrExit("")
	s.bw2.OverrideAutoChainTo(true)
	s.bw2.SetEntityFromEnvironOrExit()
	bc, err := hod.NewBW2Client(s.bw2, hoduri)
	if err != nil {
		panic(err)
	}
	s.hod = bc

	s.mux.HandleFunc(pat.Post("/add/*"), s.add)
	log.Noticef("Serving on %s...", address)
	log.Fatal(http.ListenAndServe(address, s.mux))
}

func (s *server) add(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	atomic.AddUint64(&s.num_received, 1)
	// extract the VK and path from the URI
	baseuri := strings.TrimPrefix(r.URL.String(), pat.Post("/add/").PathPrefix())
	// get the client for the corresponding vk

	var msgs map[string]SmapMessage
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&msgs); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, msg := range msgs {
		//log.Debugf("%+v", msg)
		atomic.AddUint64(&s.num_metadata, uint64(len(msg.Metadata)))
		atomic.AddUint64(&s.num_readings, uint64(len(msg.Readings)))

		if err := s.forward(msg.UUID, msg.Readings, baseuri); err != nil {
			http.Error(w, err.Error(), 500)
			return
		} else {
			log.Debugf("baseuri %s", baseuri)
		}
	}

	w.WriteHeader(200)
}

func main() {
	startServer("127.0.0.1:8001", "scratch.ns/hod")
}
