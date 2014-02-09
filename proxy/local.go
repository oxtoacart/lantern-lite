package proxy

import (
	"../s3config"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"
)

var (
	tlsConfig   *tls.Config
	enc         = base64.StdEncoding
	fallbacks   []s3config.FallbackConfig
	configMutex sync.Mutex
)

const (
	// 	authToken = "bCRAGxT2mzYVUhb6IAc2iWaXFMq8WFu0SnzhzoTfMTfUniNavV5dx3svHqxiNm83"
	//authToken              = "forrest_gump"
	x_lantern_auth_token   = "X-LANTERN-AUTH-TOKEN"
	x_random_length_header = "X_LANTERN-RANDOM-LENGTH-HEADER"
)

func StartLocal() (finished chan bool) {
	tlsConfig = &tls.Config{
		InsecureSkipVerify: true, // TODO: disable this to get security back
	}

	log.Println("Fetching fallback configuration from S3")
	doUpdateFallbacks()
	// Start continually fetching fallback information
	go updateFallbacks()

	// Run the local proxy
	finished = make(chan bool)
	go runLocal(finished)
	return
}

func updateFallbacks() {
	for {
		doUpdateFallbacks()
	}
}

func doUpdateFallbacks() {
	config := <-s3config.ConfigUpdate
	configMutex.Lock()
	defer configMutex.Unlock()
	fallbacks = config.Fallbacks
}

func getFallback() (config s3config.FallbackConfig) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if len(fallbacks) == 0 {
		log.Fatalf("No fallback configured!")
	} else {
		return fallbacks[0]
	}
	return
}

func runLocal(finished chan bool) {
	server := &http.Server{
		Addr:         ":8080",
		Handler:      http.HandlerFunc(handleLocalRequest),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("About to start local proxy at: %d", 8080)
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Unable to start local proxy: %s", err)
	}
	finished <- true
}

func handleLocalRequest(resp http.ResponseWriter, req *http.Request) {
	fallback := getFallback()
	upstreamProxy := fallback.Ip + ":" + fallback.Port
	//upstreamProxy := "192.168.1.101:62443"

	if connOut, err := tls.Dial("tcp", upstreamProxy, tlsConfig); err != nil {
		msg := fmt.Sprintf("Unable to open socket to upstream proxy: %s", err)
		respondBadGateway(resp, req, msg)
	} else {
		if connIn, _, err := resp.(http.Hijacker).Hijack(); err != nil {
			msg := fmt.Sprintf("Unable to access underlying connection from client: %s", err)
			respondBadGateway(resp, req, msg)
		} else {
			if str, err := randomLengthString(); err != nil {
				msg := fmt.Sprintf("Unable to generate random length header: %s", err)
				respondBadGateway(resp, req, msg)
			} else {
				req.Header.Set(x_random_length_header, str)
				req.Header.Set(x_lantern_auth_token, fallback.AuthToken)
				req.WriteProxy(connOut)
				pipe(connIn, connOut)
			}
		}
	}
}

func randomLengthString() (str string, err error) {
	var bLength *big.Int
	if bLength, err = rand.Int(rand.Reader, big.NewInt(100)); err != nil {
		return
	} else {
		b := make([]byte, bLength.Int64())
		if _, err = rand.Read(b); err != nil {
			return
		} else {
			str = enc.EncodeToString(b)
			return
		}
	}
}
