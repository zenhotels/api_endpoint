package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"strconv"
	"strings"
	"time"

	"os"

	"hotcore.in/skynet/skyapi"
)

var skynet = skyapi.SkyNet.New()
var reverse = &httputil.ReverseProxy{
	Transport: &http.Transport{
		Dial: func(lnet, laddr string) (net.Conn, error) {
			var host, port, hpErr = net.SplitHostPort(laddr)
			if hpErr != nil {
				return nil, hpErr
			}
			if port != "80" {
				host += ":" + port
			}
			return skynet.Dial(lnet, host)
		},
		DisableKeepAlives: true,
	},
	FlushInterval: time.Millisecond * 10,
	Director: func(req *http.Request) {
		if fwdHost == "" {
			for _, suffix := range []string{".p.hotcore.in", ".l.hotcore.in"} {
				if strings.HasSuffix(req.Host, suffix) {
					req.Host = req.Host[0 : len(req.Host)-len(suffix)]
				}
				if strings.HasSuffix(req.Host, suffix+":"+strconv.Itoa(*httpPort)) {
					req.Host = req.Host[0 : len(req.Host)-len(suffix+":"+strconv.Itoa(*httpPort))]
				}
			}
		}
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
	},
}

var httpPort = os.Getenv("HTTP_PORT")
var skyPort = os.Getenv("SKYNET_PORT")
var fwdHost = os.Getenv("FWD_HOST")

func main() {
	if skyPort == "" {
		skyPort = "10000"
	}
	if httpPort == "" {
		httpPort = "8080"
	}
	skynet.Services()

	go func() {
		if srvErr := skynet.ListenAndServe("tcp4", "0.0.0.0:"+skyPort); srvErr != nil {
			log.Panicln(srvErr)
		}
	}()
	if httpServeErr := http.ListenAndServe("0.0.0.0:"+httpPort, reverse); httpServeErr != nil {
		log.Panic(httpServeErr)
	}
}
