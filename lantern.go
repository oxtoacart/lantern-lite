package main

import (
	"./proxy"
	"github.com/oxtoacart/netutil"
	"log"
)

func main() {
	if intfs, err := netutil.ListInterfaces(); err != nil {
		log.Fatalf("Unable to list network interfaces: %s", err)
	} else {
		log.Println("Setting lantern as your proxy")
		if err := intfs.EnableHTTPProxy("127.0.0.1:8080"); err != nil {
			log.Fatalf("Unable to enable HTTP proxy: %s", err)
		} else {
			<-proxy.StartLocal()
		}
	}
}
