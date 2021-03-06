package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

type outputHttpsqs struct {
	host string
	port int

	auth           string
	flush_interval int
	debug          bool
	gzip           bool
	buffer         map[string][]byte
	client         *http.Client
	count          int
}

func (self *outputHttpsqs) Init(f map[string]string) error {
	self.host = "localhost"
	self.port = 1218
	self.flush_interval = 10
	self.gzip = true
	self.client = &http.Client{}
	self.buffer = make(map[string][]byte, 0)

	var value string

	value = f["host"]
	if len(value) > 0 {
		self.host = value
	}

	value = f["port"]
	if len(value) > 0 {
		self.port, _ = strconv.Atoi(value)
	}

	value = f["auth"]
	if len(value) > 0 {
		self.auth = value
	}

	value = f["flush_interval"]
	if len(value) > 0 {
		self.flush_interval, _ = strconv.Atoi(value)
	}

	value = f["gzip"]
	if len(value) > 0 {
		if value == "off" {
			self.gzip = false
		}
	}

	return nil
}

func (self *outputHttpsqs) Run(runner OutputRunner) error {

	tick := time.NewTicker(time.Second * time.Duration(self.flush_interval))

	for {
		select {
		case <-tick.C:
			{
				if len(self.buffer) > 0 {
					self.flush()
				}
			}
		case pack := <-runner.InChan():
			{
				b, err := json.Marshal(pack.Msg.Data)

				if err != nil {
					log.Println("json.Marshal:", err)
					pack.Recycle()
					continue
				}

				if len(self.buffer) == 0 {
					self.buffer[pack.Msg.Tag] = append(self.buffer[pack.Msg.Tag], byte('['))
				} else if len(self.buffer) > 0 {
					self.buffer[pack.Msg.Tag] = append(self.buffer[pack.Msg.Tag], byte(','))
				}

				self.count++
				self.buffer[pack.Msg.Tag] = append(self.buffer[pack.Msg.Tag], b...)
				pack.Recycle()
			}
		}
	}
}

func (self *outputHttpsqs) flush() {
	for k, v := range self.buffer {
		url := fmt.Sprintf("http://%s:%d/?name=%s&opt=put&auth=%s", self.host, self.port, k, self.auth)

		v = append(v, byte(']'))
		var buf bytes.Buffer
		var req *http.Request

		if self.gzip == true {
			gzw := gzip.NewWriter(&buf)
			gzw.Write([]byte(v))
			gzw.Close()
			req, _ = http.NewRequest("POST", url, bytes.NewReader(buf.Bytes()))
		} else {
			req, _ = http.NewRequest("POST", url, bytes.NewReader([]byte(v)))
		}

		req.Header.Add("Content-Encoding", "gzip")
		req.Header.Add("Content-Type", "application/json")

		log.Println("url:", url, "count:", self.count, "length:", len(v), "gziped:", buf.Len())

		resp, err := self.client.Do(req)
		if err != nil {
			log.Println("post failed:", err)
			continue
		}

		v, _ := ioutil.ReadAll(resp.Body)
		log.Println("StatusCode:", resp.StatusCode, string(v), "Pos:", resp.Header.Get("Pos"))

		resp.Body.Close()
		self.buffer[k] = self.buffer[k][0:0]
		delete(self.buffer, k)
		self.count = 0
	}
}

func init() {
	RegisterOutput("httpsqs", func() interface{} {
		return new(outputHttpsqs)
	})
}
