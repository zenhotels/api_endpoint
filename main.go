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

	"strings"

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
	Upstream    *url.URL
	Director    func(*http.Request)
	DialTimeout time.Duration
	VHost       []string
	Listen      string
}

func mkReverse(c reverseConf) *httputil.ReverseProxy {
	var reverse = &httputil.ReverseProxy{
		FlushInterval: time.Millisecond * 10,
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = c.Upstream.Host
			req.Host = c.Upstream.Host

			if c.Director != nil {
				c.Director(req)
			}
		},
	}
	switch c.Upstream.Scheme {
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
		log.Panicln("Unsupported scheme", c.Upstream.Scheme)
	}
	return reverse
}

var httpPort = os.Getenv("HTTP_PORT")
var skyPort = os.Getenv("SKYNET_PORT")
var sysHost = strings.Split(os.Getenv("SYSHOST"), ",")

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
			continue
		}
		var service, param, value = envParsed[1], envParsed[2], envParsed[3]
		var srv = services[service]
		switch param {
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
			srv.VHost = strings.Split(value, ",")
		default:
			log.Panicln("Error while parsing in unknown param in", envQ, param)
		}
		services[service] = srv
	}

	var hs = make(HostSwitch)
	for srvName, srvConf := range services {
		if srvConf.Upstream.Scheme == "" {
			srvConf.Upstream.Scheme = "shttp"
		}
		if srvConf.Upstream == nil {
			log.Panicln("UPSTREAM not configured for", srvName)
		}
		if srvConf.DialTimeout == 0 {
			srvConf.DialTimeout = time.Second * 10
		}
		if len(srvConf.VHost) == 0 {
			log.Panicln("HOST not configured for", srvName)
		}
		services[srvName] = srvConf
		var vHosts = srvConf.VHost
		for _, vHost := range srvConf.VHost {
			for _, vSysHost := range sysHost {
				vHosts = append(vHosts, vHost+"."+vSysHost)
			}
		}
		var r = mkReverse(srvConf)
		for _, vHost := range vHosts {
			if hs[vHost] != nil {
				log.Panicln("Multiple usage of HOST", vHost)
			}
			hs[vHost] = r
			log.Println("Serving HTTP for", vHost)
		}
		for _, vHost := range srvConf.VHost {
			for _, vSysHost := range sysHost {
				var srv = vHost + "." + vSysHost
				var skyL, skyLErr = skynet.Bind("", srv)
				if skyLErr != nil {
					log.Panicln("Error while binding skynet service", srv)
				}
				log.Println("Serving SHTTP for", srv)
				go http.Serve(skyL, r)
			}
		}
	}

	if srvErr := skynet.ListenAndServe("tcp4", "0.0.0.0:"+skyPort); srvErr != nil {
		log.Panicln(srvErr)
	}

	if httpServeErr := http.ListenAndServe("0.0.0.0:"+httpPort, hs); httpServeErr != nil {
		log.Panic(httpServeErr)
	}
}
