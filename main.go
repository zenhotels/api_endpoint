package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"strings"
	"time"

	"fmt"
	"github.com/satori/go.uuid"
	"hotcore.in/skynet/skyapi"
	"os"
	"encoding/binary"
)

var skynet = skyapi.SkyNet.Client().New()
var skyserv = skyapi.SkyNet.Server().New()
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
				if strings.HasSuffix(req.Host, suffix+":"+httpPort) {
					req.Host = req.Host[0 : len(req.Host)-len(suffix+":"+httpPort)]
				}
			}
		} else {
			req.Host = fwdHost
		}

		var session = req.URL.Query().Get("session")
		if session != "" {
			var sessuid, decErr = uuid.FromString(session)
			if decErr == nil {
				req.Host = fmt.Sprintf(
					"%s:%d",
					skyapi.Uint2Host(binary.BigEndian.Uint64(sessuid.Bytes()[0:8])),
					13337,
				)
			}
		}

		req.URL.Scheme = "http"
		req.URL.Host = req.Host
	},
}

var httpPort = os.Getenv("HTTP_PORT")
var skyPort = os.Getenv("SKYNET_PORT")
var fwdHost = os.Getenv("FWD_HOST")
var srvId = os.Getenv("SERVICE_ID")

func main() {
	if skyPort == "" {
		skyPort = "10000"
	}
	if httpPort == "" {
		httpPort = "8080"
	}
	skynet.Services()

	var skyL, skyLErr = skyserv.Bind("", srvId)
	if skyLErr != nil {
		log.Panicln(skyLErr)
	}

	go http.Serve(skyL, reverse)
	go func() {
		if srvErr := skyserv.ListenAndServe("tcp4", "0.0.0.0:"+skyPort); srvErr != nil {
			log.Panicln(srvErr)
		}
	}()
	if httpServeErr := http.ListenAndServe("0.0.0.0:"+httpPort, reverse); httpServeErr != nil {
		log.Panic(httpServeErr)
	}
}
