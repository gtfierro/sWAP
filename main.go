package main

import (
	"github.com/codegangsta/cli"
	"github.com/op/go-logging"
	"github.com/pkg/errors"
	"os"
)

// logger
var log *logging.Logger

const bufferFile = ".sWAP.db"

// set up logging facilities
func init() {
	log = logging.MustGetLogger("sWAP")
	var format = "%{color}%{level} %{time:Jan 02 15:04:05} %{shortfile}%{color:reset} ▶ %{message}"
	var logBackend = logging.NewLogBackend(os.Stderr, "", 0)
	logBackendLeveled := logging.AddModuleLevel(logBackend)
	logging.SetBackend(logBackendLeveled)
	logging.SetFormatter(logging.MustStringFormatter(format))
}

func doServer(c *cli.Context) error {
	address := c.String("address")
	store := newStore(bufferFile)
	startServer(address, store)
	return nil
}

func doRegister(c *cli.Context) error {
	if c.NArg() == 0 {
		return errors.New("Need to supply an entity file name")
	}
	filename := c.Args().Get(0)
	store := newStore(bufferFile)
	if vk, err := store.addEntityFile(filename); err == nil {
		log.Noticef("Stored key with VK= %s", vk)
	} else {
		return err
	}
	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "sWAP"
	app.Usage = "Simple WAVE Acclimation Proxy"
	app.Version = "0.1"

	app.Commands = []cli.Command{
		{
			Name:   "server",
			Usage:  "Start the proxy server",
			Action: doServer,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "address,a",
					Value: "localhost:8078",
					Usage: "Address to listen on",
				},
			},
		},
		{
			Name:   "register",
			Usage:  "Register an entity so it can be used",
			Action: doRegister,
		},
	}
	app.Run(os.Args)
}
