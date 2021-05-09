/*
Gondola Bookmarker
This gondola helper bookmarks where you're up to in a movie/episode so you can resume later.
It is designed to store everything in memory primarily, and only write to disk intermittently so we
don't wear out SSDs by writing every couple seconds.

To install:
go get -v github.com/chrishulbert/gondola-bookmarker
test it: ~/go/bin/gondola-bookmarker
sudo nano /lib/systemd/system/gondola-bookmarker.service
[Unit]
Description=GondolaBookmarker

[Service]
PIDFile=/tmp/gondola-bookmarker.pid
User=gondola
Group=gondola
ExecStart=/home/gondola/go/bin/gondola-bookmarker

[Install]
WantedBy=multi-user.target

sudo systemctl enable gondola-bookmarker
sudo systemctl start gondola-bookmarker
systemctl status gondola-bookmarker
test:
curl localhost:35248
curl -X POST -F "item=abc" -F "time=1234" localhost:35248/bookmark/set
curl "localhost:35248/bookmark/get?item=abc"
test from other mac: curl http://gondola.local:35248/
*/

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"time"
)

var bookmarks = make(map[string]int)
var needs_saving = false
var mutex sync.RWMutex

func expandTilde(path string) string {
	usr, _ := user.Current()
	homeDir := usr.HomeDir
	return strings.Replace(path, "~", homeDir, -1)
}

func bookmarksPath() string {
	return expandTilde("~/.gondolaBookmarks.json")
}

func saveEveryNowAndAgain() {
	for {
		time.Sleep(time.Hour)

		// Do the minimum inside the lock: Check if has changed, marshal if so.
		var json_if_we_want_to_write []byte = nil // Slices can be nil in go.
		mutex.RLock()
		if needs_saving {
			json, err := json.Marshal(bookmarks)
			if err != nil {
				log.Println(fmt.Sprintf("Error serialising: %s", err.Error()))
			} else {
				json_if_we_want_to_write = json
				needs_saving = false
			}
		}
		mutex.RUnlock()

		// Do the slow write outside the lock.
		if json_if_we_want_to_write != nil {
			path := bookmarksPath()
			err := ioutil.WriteFile(path, json_if_we_want_to_write, os.ModePerm)
			if err != nil {
				log.Println(fmt.Sprintf("Error writing: %s", err.Error()))
			}
		}
	}
}

func load() {
	file, err := ioutil.ReadFile(bookmarksPath())
	if err != nil {
		return
	}
	var items map[string]int
	err = json.Unmarshal(file, &items)
	if err != nil {
		return
	}
	if items != nil {
		bookmarks = items
	}
}

func main() {
	load()	

	go saveEveryNowAndAgain()

	http.HandleFunc("/bookmark/set", func(w http.ResponseWriter, r *http.Request) {
		item := r.FormValue("item")
		time, _ := strconv.Atoi(r.FormValue("time"))
		mutex.Lock()
		bookmarks[item] = time
		needs_saving = true
		mutex.Unlock()
	})
	http.HandleFunc("/bookmark/get", func(w http.ResponseWriter, r *http.Request) {
		// Get the bookmark:
		item := r.FormValue("item")
		mutex.RLock()
		time := bookmarks[item]
		mutex.RUnlock()
		// Respond to the caller:
		type myresponse struct {
			Time int `json:"time"`
		}
		response := myresponse{Time: time}
		json, err := json.Marshal(response)
		if err != nil {
			log.Println(fmt.Sprintf("Error serialising: %s", err.Error()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(json)
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from Gondola Bookmarker!\n")
	})
	err := http.ListenAndServe(":35248", nil)
	if err != nil {
		log.Fatal(err)
	}
}
