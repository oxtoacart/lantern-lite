/*
Package s3config encapsulates logic for fetching configuration updates from an Amazon S3 url defined
in the file .lantern-configurl.txt in whichever folder lantern is running.
*/
package s3config

import (
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"time"
)

const (
	urlfile = ".lantern-configurl.txt"                                  // file from which to get url
	s3base  = "https://s3-ap-southeast-1.amazonaws.com/lantern-config/" // base url for accessing s3
)

var (
	ConfigUpdate chan S3Config // channel on which we notify listener of config updates
	s3url        string        // the url from which we'll fetch updates
	minPoll      = 5           // minimum polling interval in minutes (value will change based on fetched config)
	maxPoll      = 15          // maximum polling interval in minutes  (value will change based on fetched config)
)

/*
S3Config represents the configuration provided by S3.
*/
type S3Config struct {
	SerialNo   int               `json:"serial_no"`
	Controller string            `json:"controller"`
	MinPoll    int               `json:"minpoll"`
	MaxPoll    int               `json:"maxpoll"`
	Fallbacks  []*FallbackConfig `json:"fallbacks"`
}

/*
FallbackConfig represents the configuration of a fallback proxy.
*/
type FallbackConfig struct {
	Ip        string `json:"ip"`
	Port      string `json:"port"`
	Protocol  string `json:"protocol"`
	AuthToken string `json:"auth_token"`
	Cert      string `json:"cert"`
	X509Cert  *x509.Certificate
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

/*
fetch continually fetches updates from s3 and publishes them on the ConfigUpdate channel.
*/
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
					certFailure := false
					for _, fallback := range config.Fallbacks {
						if cert, err := parseCert(fallback.Cert); err != nil {
							log.Printf("Unable to parse cert: %s", err)
							certFailure = true
							break
						} else {
							fallback.X509Cert = cert
						}
					}
					if !certFailure {
						ConfigUpdate <- config
					}
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

/*
parseCert parses a PEM encoded certificate into an x509.Certificate object.
*/
func parseCert(certData string) (cert *x509.Certificate, err error) {
	var block *pem.Block
	block, _ = pem.Decode([]byte(certData))
	if block == nil {
		err = fmt.Errorf("No PEM encoded certificate found")
		return
	}
	cert, err = x509.ParseCertificate(block.Bytes)
	return
}
