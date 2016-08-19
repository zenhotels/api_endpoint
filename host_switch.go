package main

import (
	"fmt"
	"net/http"
)

type HostSwitch map[string]http.Handler

func Index(w http.ResponseWriter, r *http.Request) {
	var srvList = skynet.Services()
	for _, srv := range srvList {
		fmt.Fprintln(w, srv)
	}
	var nodeList = skynet.Routes()
	for _, srv := range nodeList {
		fmt.Fprintln(w, srv)
	}
}

// Implement the ServerHTTP method on our new type
func (hs HostSwitch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if a http.Handler is registered for the given host.
	// If yes, use it to handle the request.
	if handler := hs[r.Host]; handler != nil {
		handler.ServeHTTP(w, r)
	} else if handler = hs["p."+sysHost[0]]; handler != nil {
		handler.ServeHTTP(w, r)
	} else {
		http.DefaultServeMux.ServeHTTP(w, r)
	}
}
