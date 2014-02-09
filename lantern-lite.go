/*
lantern-lite is a slimmed down Lantern that fetches its fallback information from the usual S3 mechanism
and then proxies traffic for you on port 8080.
*/
package main

import (
	"./proxy"
	"github.com/oxtoacart/netutil"
	"log"
	"os"
	"os/signal"
)

/*
main() is the main entry point into the lantern application.
*/
func main() {
	if intfs, err := netutil.ListInterfaces(); err != nil {
		log.Fatalf("Unable to list network interfaces: %s", err)
	} else {
		log.Println("Setting lantern-lite as your proxy")
		if err := intfs.EnableHTTPProxy("127.0.0.1:8080"); err != nil {
			log.Fatalf("Unable to set lantern-lite as your proxy: %s", err)
		} else {
			onShutdown(func() {
				log.Println("Unsetting lantern-lite as your proxy")
				intfs.DisableHTTPProxy()
			})
			<-proxy.StartLocal()
		}
	}
}

func onShutdown(fn func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		fn()
		os.Exit(0)
	}()
}
