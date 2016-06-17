package main

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"time"

	"github.com/satori/go.uuid"

	"os"

	"net/url"

	"encoding/binary"
	"fmt"

	"regexp"

	"hotcore.in/skynet/skyapi"
)

var skynet = skyapi.SkyNet.New()

func SessionBasedDirector(sessLocator func(*http.Request) string, vport string) func(*http.Request) {
	return func(req *http.Request) {
		var session = sessLocator(req)
		if session != "" {
			var sessuid, decErr = uuid.FromString(session)
			if decErr == nil {
				req.Host = fmt.Sprintf(
					"%s:%s",
					skyapi.Uint2Host(binary.BigEndian.Uint64(sessuid.Bytes()[0:8])),
					vport,
				)
			}
		}
	}
}

func SessionLocatorQuery(pName string) func(*http.Request) string {
	return func(req *http.Request) string {
		return req.URL.Query().Get(pName)
	}
}

type reverseConf struct {
	Protocol    string
	Upstream    *url.URL
	Director    func(*http.Request)
	DialTimeout time.Duration
	VHost       string
}

func mkReverse(c reverseConf) *httputil.ReverseProxy {
	var reverse = &httputil.ReverseProxy{
		FlushInterval: time.Millisecond * 10,
		Director: func(req *http.Request) {
			req.URL.Scheme = c.Upstream.Scheme
			req.URL.Host = c.Upstream.Host

			if c.Director != nil {
				c.Director(&req)
			}
		},
	}
	switch c.Protocol {
	case "shttp":
		var dialer = &net.Dialer{
			Timeout:   c.DialTimeout,
			DualStack: false,
		}
		reverse.Transport = &http.Transport{
			Dial:              dialer.Dial,
			DisableKeepAlives: true,
		}
	case "http":
		reverse.Transport = &http.Transport{
			Dial: func(lnet, laddr string) (net.Conn, error) {
				var host, port, hpErr = net.SplitHostPort(laddr)
				if hpErr != nil {
					return nil, hpErr
				}
				if port != "80" {
					host += ":" + port
				}
				return skynet.DialTimeout(lnet, host, c.DialTimeout)
			},
			DisableKeepAlives: true,
		}
	default:
		log.Panicln("Unsupported scheme", c.Protocol)
	}
}

var httpPort = os.Getenv("HTTP_PORT")
var skyPort = os.Getenv("SKYNET_PORT")

var services = map[string]reverseConf{}
var srvRe = regexp.MustCompilePOSIX("SRV_([A-Z0-9_]*)_([A-Z0-9_]*)=(.*)")

func main() {
	if skyPort == "" {
		skyPort = "10000"
	}
	if httpPort == "" {
		httpPort = "8080"
	}
	skynet.Services()

	for _, envQ := range os.Environ() {
		var envParsed = srvRe.FindStringSubmatch(envQ)
		if len(envParsed) == 0 {
			log.Println("Skipping environment variable", envQ)
		}
		var service, param, value = envParsed[1], envParsed[2], envParsed[3]
		var srv = services[service]
		switch param {
		case "PROTOCOL":
			srv.Protocol = value
		case "UPSTREAM":
			var upstream, upErr = url.Parse(value)
			if upErr != nil {
				log.Panicln("Error while UPSTREAM parsing in", envQ, upErr)
			}
			srv.Upstream = upstream
		case "SESSION":
			var sessParams, sessErr = url.ParseQuery(value)
			if sessErr != nil {
				log.Panicln("Error while SESSION parsing in", envQ, sessErr)
			}
			srv.Director = SessionBasedDirector(
				SessionLocatorQuery(sessParams.Get("key")),
				sessParams.Get("vport"),
			)
		case "TIMEOUT":
			var duration, dParseErr = time.ParseDuration(value)
			if dParseErr != nil {
				log.Panicln("Error while TIMEOUT parsing in", envQ, dParseErr)
			}
			srv.DialTimeout = duration
		case "HOST":
			srv.VHost = value
		default:
			log.Panicln("Error while parsing in unknown param in", envQ, param)
		}
		services[service] = srv
	}

	for srvName, srvConf := range services {
		if srvConf.Protocol == "" {
			srvConf.Protocol = "shttp"
		}
		if srvConf.Upstream == nil {
			log.Panicln("UPSTREAM not configured in", srvName)
		}
		if srvConf.DialTimeout == 0 {
			srvConf.DialTimeout = time.Second * 10
		}
		services[srvName] = srvConf
	}

	go http.Serve(skyL, reverse)
	go func() {
		if srvErr := skyserv.ListenAndServe("tcp4", "0.0.0.0:"+skyPort); srvErr != nil {
			log.Panicln(srvErr)
		}
	}()

	var skyL, skyLErr = skyserv.Bind("", srvId)
	if skyLErr != nil {
		log.Panicln(skyLErr)
	}

	if httpServeErr := http.ListenAndServe("0.0.0.0:"+httpPort, reverse); httpServeErr != nil {
		log.Panic(httpServeErr)
	}
}
