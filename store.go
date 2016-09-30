package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/boltdb/bolt"
	"github.com/immesys/bw2/objects"
	"github.com/pkg/errors"
	bw2 "gopkg.in/immesys/bw2bind.v5"
)

var entityBucket = []byte("entity")

// stores our entities and allows us to pull the BW2Clients using the VKs
type entityStore struct {
	filename string
	// local file database that stores entities
	db     *bolt.DB
	dbLock sync.Mutex
	// cache of active BW2Clients for each VK
	clients map[string]*bw2.BW2Client
	sync.RWMutex
}

// create a new entity store at the given filename
func newStore(filename string) *entityStore {
	db, err := bolt.Open(filename, 0600, nil)
	if err != nil {
		log.Fatal(errors.Wrap(err, "Could not open database file"))
	}

	s := &entityStore{
		db:       db,
		filename: filename,
		clients:  make(map[string]*bw2.BW2Client),
	}

	s.scanAndLoadVKs()
	s.dbLock.Lock()
	defer s.dbLock.Unlock()
	return s
}

func (s *entityStore) waitForSignal() {
	var err error
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	go func() {
		for {
			log.Warning("Waiting for signal")
			<-c
			log.Warning("Got signal (lock)")
			s.db.Close()
			s.dbLock.Lock()
			<-c
			log.Warning("Got signal (unlock)")
			if s.db, err = bolt.Open(s.filename, 0600, nil); err != nil {
				log.Error(err)
			}
			s.scanAndLoadVKs()
			s.dbLock.Unlock()
		}
	}()
}

func (s *entityStore) scanAndLoadVKs() {
	s.Lock()
	defer s.Unlock()
	s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(entityBucket)
		if err != nil {
			return errors.Wrap(err, "Could not create entity bucket")
		}
		// loop through the bucket and create clients for each of the known keys
		b.ForEach(func(vk, contents []byte) error {
			client := bw2.ConnectOrExit("")
			vk2, err := client.SetEntity(contents)
			if err != nil {
				log.Error(errors.Wrap(err, "Could not set entity"))
				return nil
			}
			vk_string := base64.URLEncoding.EncodeToString(vk)
			if vk_string != vk2 {
				log.Error(errors.Wrapf(err, "Retrieved vk %s did not match vk from router %s", vk_string, vk2))
				return nil
			}
			s.clients[vk_string] = client
			log.Infof("Loaded vk %s", vk_string)
			return nil
		})
		return nil
	})
}

// Add entity from the given file name.
// The file contents get stored in the entity bucket with the public key (vk) as the key.
// Returns the vk of the key on success
func (s *entityStore) addEntityFile(filename string) (string, error) {
	//TODO: send a user-defined signal to any running server process to load in the new VK
	// read the file to get its contents; this way, we can just store the
	// bytes instead of having to keep the file intact
	contents, err := ioutil.ReadFile(filename)
	fileType := contents[0]
	contents = contents[1:]
	if err != nil {
		return "", errors.Wrapf(err, "Could not read entity file %s", filename)
	}

	// parse the contents of the file to extract the vk
	ro, err := objects.NewEntity(int(fileType), contents)
	if err != nil {
		return "", errors.Wrap(err, "Could not parse entity")
	}
	entity := ro.(*objects.Entity)
	vk := entity.GetVK()
	vk_string := base64.URLEncoding.EncodeToString(vk)

	fmt.Println("update")
	s.dbLock.Lock()
	defer s.dbLock.Unlock()
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(entityBucket)
		return b.Put(vk, contents)
	})

	fmt.Println("open file")
	return vk_string, err
}

func (s *entityStore) getClientForVK(vk string) *bw2.BW2Client {
	s.RLock()
	defer s.RUnlock()
	return s.clients[vk]
}
