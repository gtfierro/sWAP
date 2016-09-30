package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pkg/errors"
	"goji.io"
	"goji.io/pat"
	"goji.io/pattern"
	"golang.org/x/net/context"
	bw2 "gopkg.in/immesys/bw2bind.v5"
)

type server struct {
	mux   *goji.Mux
	store *entityStore
}

func startServer(address string, store *entityStore, pidfile string) {
	var (
		f   *os.File
		err error
	)
	// write the PID to the file
	pid := os.Getpid()

	if f, err = os.Create(pidfile); err != nil {
		log.Fatal(errors.Wrap(err, "Cannot write PID file"))
	} else if _, err = f.WriteString(fmt.Sprintf("%d", pid)); err != nil {
		log.Fatal(errors.Wrap(err, "Cannot write PID file"))
	}
	if err = f.Close(); err != nil {
		log.Fatal(errors.Wrap(err, "Cannot write PID file"))
	}

	s := &server{
		mux:   goji.NewMux(),
		store: store,
	}
	s.mux.HandleFuncC(pat.Post("/add/:vk/uri/*"), s.add)
	log.Noticef("Serving on %s...", address)
	log.Fatal(http.ListenAndServe(address, s.mux))
}

func (s *server) add(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// extract the VK and path from the URI
	vk := pat.Param(ctx, "vk")
	baseuri := strings.TrimPrefix(pattern.Path(ctx), "/")
	// get the client for the corresponding vk
	client := s.store.getClientForVK(vk)
	if client == nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("No bw2 client found for vk %s", vk)))
		return
	}

	// pull the posted JSON out of the sMAP message
	messages, err := handleJSON(r.Body)
	if err != nil {
		log.Errorf("Error handling JSON %s", err)
		w.WriteHeader(400)
		w.Write([]byte(err.Error()))
		return
	}

	// persist the metadata we extract
	for _, msg := range messages {
		for k, v := range msg.Metadata {
			vs, ok := v.(string)
			if !ok {
				vs = fmt.Sprintf("%v", v)
			}
			uri := buildURI(baseuri, msg.Path)
			client.SetMetadata(uri, k, vs)
		}
		if len(msg.Readings) == 0 {
			log.Debug(msg.Path)
		}
	}

	// find timeseries data, form POs, and publish
	for _, msg := range messages {
		if len(msg.Readings) > 0 {
			uri := buildURI(baseuri, msg.Path)
			po := TimeseriesReading{UUID: string(msg.UUID)}
			for _, rdg := range msg.Readings {
				po.Time = int64(rdg.GetTime())
				po.Value = rdg.GetValue().(float64)
				if err := client.Publish(&bw2.PublishParams{
					URI:            uri,
					PayloadObjects: []bw2.PayloadObject{po.ToMsgPackBW()},
				}); err != nil {
					log.Errorf("Error publishing message %s", err)
					w.WriteHeader(400)
					w.Write([]byte(err.Error()))
					return
				}
			}
			log.Debug("Timeseries", uri)
		}
	}

	w.WriteHeader(200)
}

func handleJSON(r io.Reader) (decoded TieredSmapMessage, err error) {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	err = decoder.Decode(&decoded)
	for path, msg := range decoded {
		msg.Path = path
	}
	return
}

func buildURI(baseuri, uri string) string {
	uri = baseuri + "/" + uri
	uri = strings.Replace(uri, "//", "/", -1)
	uri = strings.TrimSuffix(uri, "/")
	return uri
}

type TimeseriesReading struct {
	UUID  string
	Time  int64
	Value float64
}

func (msg TimeseriesReading) ToMsgPackBW() (po bw2.PayloadObject) {
	po, _ = bw2.CreateMsgPackPayloadObject(bw2.FromDotForm("2.0.9.1"), msg)
	return
}
