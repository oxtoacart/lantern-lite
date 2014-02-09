/*
Package proxy includes a local proxy that proxies traffic from the user's browser
to a remote Lantern proxy.
*/
package proxy

import (
	"../s3config"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"
)

/*
Fallback encapsulates the configuration for a fallback proxy along with the tls configuration used to communicate with it.
*/
type Fallback struct {
	s3config.FallbackConfig
	tlsConfig *tls.Config
}

var (
	enc            = base64.StdEncoding // Used for Base64 encoding stuff
	fallbacks      []Fallback           // All configured fallbacks
	fallbacksMutex sync.Mutex           // Used to synchronize access to fallbacks
)

const (
	x_lantern_auth_token   = "X-LANTERN-AUTH-TOKEN"
	x_random_length_header = "X_LANTERN-RANDOM-LENGTH-HEADER"
)

/*
StartLocal() starts the local proxy server.
*/
func StartLocal() (finished chan bool) {
	log.Println("Fetching fallback configuration from S3")
	doUpdateFallbacks()
	// Start continually fetching fallback information
	go updateFallbacks()

	// Run the local proxy
	finished = make(chan bool)
	go runLocal(finished)
	return
}

/*
updateFallbacks() keeps updating the fallbacks list as new configuration information becomes available.
*/
func updateFallbacks() {
	for {
		doUpdateFallbacks()
	}
}

/*
doUpdateFallbacks waits for a new configuration from s3config and then updates the fallbacks list.
*/
func doUpdateFallbacks() {
	config := <-s3config.ConfigUpdate
	fallbacksMutex.Lock()
	defer fallbacksMutex.Unlock()
	fallbacks = make([]Fallback, len(config.Fallbacks))
	for i, fallbackConfig := range config.Fallbacks {
		tlsConfig := &tls.Config{
			RootCAs: x509.NewCertPool(),
			// I have to do this because our current fallback certificates don't contain IP SANs, see https://github.com/getlantern/lantern/issues/1373
			InsecureSkipVerify: true,
		}
		tlsConfig.RootCAs.AddCert(fallbackConfig.X509Cert)
		fallbacks[i] = Fallback{
			FallbackConfig: *fallbackConfig,
			tlsConfig:      tlsConfig,
		}
	}
}

/*
getFallback() gets the first fallback from the fallbacks list

TODO: cycle through fallbacks when multiple are available.
*/
func getFallback() (fallback Fallback) {
	fallbacksMutex.Lock()
	defer fallbacksMutex.Unlock()
	if len(fallbacks) == 0 {
		log.Fatalf("No fallback configured!")
	} else {
		return fallbacks[0]
	}
	return
}

/*
runLocal rnus the http server for the local proxy.
*/
func runLocal(finished chan bool) {
	server := &http.Server{
		Addr:         "127.0.0.1:8080",
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

/*
handleLocalRequest handles local requests (e.g. from web browser) and dispatches them to a remote fallback.
*/
func handleLocalRequest(resp http.ResponseWriter, req *http.Request) {
	fallback := getFallback()
	upstreamAddr := fallback.Ip + ":" + fallback.Port

	if connOut, err := tls.Dial("tcp", upstreamAddr, fallback.tlsConfig); err != nil {
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
				// Send the initial request on to the downstream proxy
				req.Header.Set(x_random_length_header, str)
				req.Header.Set(x_lantern_auth_token, fallback.AuthToken)
				req.WriteProxy(connOut)
				// Then pipe the connection
				pipe(connIn, connOut)
			}
		}
	}
}

/*
randomLengthString generates a random length string up to a little over 100 characters in length.
*/
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
