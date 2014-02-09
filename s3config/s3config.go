package s3config

import (
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"time"
)

const (
	urlfile = ".lantern-configurl.txt"
	s3base  = "https://s3-ap-southeast-1.amazonaws.com/lantern-config/"
)

var (
	ConfigUpdate chan S3Config
	s3url        string
	minPoll      = 5
	maxPoll      = 15
)

type S3Config struct {
	SerialNo   int              `json:"serial_no"`
	Controller string           `json:"controller"`
	MinPoll    int              `json:"minpoll"`
	MaxPoll    int              `json:"maxpoll"`
	Fallbacks  []FallbackConfig `json:"fallbacks"`
}

type FallbackConfig struct {
	Ip        string `json:"ip"`
	Port      string `json:"port"`
	Protocol  string `json:"protocol"`
	AuthToken string `json:"auth_token"`
	Cert      string `json:"cert"`
}

func init() {
	ConfigUpdate = make(chan S3Config)
	if bytes, err := ioutil.ReadFile(urlfile); err != nil {
		log.Fatal("Unable to read .lantern-configurl.txt.  Make sure that you have a .lantern-configurl.txt in the folder where you're running lantern. %s", err)
	} else {
		s3url = s3base + string(bytes) + "/config.json"
		go fetch()
	}
}

func fetch() {
	if resp, err := http.Get(s3url); err != nil {
		log.Printf("Unable to fetch s3 configuration: %s", err)
	} else {
		defer resp.Body.Close()
		if body, err := ioutil.ReadAll(resp.Body); err != nil {
			log.Printf("Unable to read s3 configuration from response: %s", err)
		} else {
			if resp.StatusCode != 200 {
				log.Printf("Unexpected response status: %d", resp.StatusCode)
				log.Printf("URL was: %s", s3url)
				log.Printf("--------- Body was: -----------\n%s\n-----------------", body)
			} else {
				config := S3Config{}
				if err := json.Unmarshal(body, &config); err != nil {
					log.Printf("Unable to decode s3 configuration; %s", err)
				} else {
					minPoll = config.MinPoll
					maxPoll = config.MaxPoll
					ConfigUpdate <- config
				}
			}
		}
	}
	maxRandomVal := big.NewInt(int64(maxPoll - minPoll))
	if randomVal, err := rand.Int(rand.Reader, maxRandomVal); err != nil {
		log.Fatalf("Unable to update poll interval: %s", err)
	} else {
		interval := randomVal.Int64() + int64(minPoll)
		time.Sleep(time.Duration(interval) * time.Minute)
	}
}
