package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/satori/go.uuid"

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
				req.URL.Host = req.Host
			}
		}
	}
}

func SessionLocatorQuery(pName string) func(*http.Request) string {
	return func(req *http.Request) string {
		return req.URL.Query().Get(pName)
	}
}

func JoinSkipEmpty(sep string, s ...string) string {
	var sL = make([]string, 0, len(s))
	for _, si := range s {
		if si == "" {
			continue
		}
		sL = append(sL, si)
	}
	return strings.Join(sL, sep)
}

type reverseConf struct {
	Upstream    *url.URL
	Director    func(*http.Request)
	DialTimeout time.Duration
	VHost       []string
	Listen      string
}

type keyConf struct {
	ID      string
	VSrvMap map[string]string
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
	case "http":
		var dialer = &net.Dialer{
			Timeout:   c.DialTimeout,
			DualStack: false,
		}
		reverse.Transport = &http.Transport{
			Dial:              dialer.Dial,
			DisableKeepAlives: true,
		}
	case "shttp":
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
var apiKeys = map[string]keyConf{}

var srvRe = regexp.MustCompilePOSIX("SRV_([A-Z0-9_]*)_([A-Z0-9_]*)=(.*)")
var keyRe = regexp.MustCompilePOSIX("KEY_([A-Z0-9_]*)_([A-Z0-9_]*)=(.*)")
var stageRe = regexp.MustCompilePOSIX("STAGE_([A-Z0-9_]*)_([A-Z0-9_]*)=(.*)")

func main() {
	if skyPort == "" {
		skyPort = "10000"
	}
	if httpPort == "" {
		httpPort = "8080"
	}
	skynet.Services()
	var httpBind = "0.0.0.0:" + httpPort

	apiKeys["common"] = keyConf{}

	for _, envQ := range os.Environ() {
		var envParsed = srvRe.FindStringSubmatch(envQ)
		var apiKeyParsed = keyRe.FindStringSubmatch(envQ)
		var stageKeyParsed = stageRe.FindStringSubmatch(envQ)
		if len(envParsed) > 0 {
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
			continue
		}
		if len(apiKeyParsed) > 0 {
			var keyName, param, value = apiKeyParsed[1], apiKeyParsed[2], apiKeyParsed[3]
			var key = apiKeys[keyName]
			switch param {
			case "ID":
				key.ID = value
			default:
				log.Panicln("Error while parsing in unknown param in", envQ, param)
			}
			apiKeys[keyName] = key
			continue
		}
		if len(stageKeyParsed) > 0 {
			var keyName, param, value = stageKeyParsed[1], stageKeyParsed[2], stageKeyParsed[3]
			var key = apiKeys[keyName]
			if key.VSrvMap == nil {
				key.VSrvMap = make(map[string]string)
			}
			key.VSrvMap[param] = value
			apiKeys[keyName] = key
		}
		log.Println("Skipping environment variable", envQ)
	}

	var hMap = map[string]http.Handler{}

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
		hMap[srvName] = mkReverse(srvConf)
	}

	var hs = make(HostSwitch)

	for apiId, apiKey := range apiKeys {
		for srvName, srvConf := range services {
			var r = hMap[srvName]
			if override, found := apiKey.VSrvMap[srvName]; found {
				r = hMap[override]
			}
			if r == nil {
				log.Panicln("No handler for", apiId, srvName)
			}
			for _, vHost := range srvConf.VHost {
				for _, vSysHost := range sysHost {
					var vHost = JoinSkipEmpty(".", vHost, apiKey.ID, vSysHost)
					if hs[vHost] != nil {
						log.Panicln("Multiple usage of HOST", vHost)
					}
					hs[vHost] = r
					log.Println("Serving HTTP for", vHost, "on", httpBind)

					var skyL, skyLErr = skynet.Bind("", vHost)
					if skyLErr != nil {
						log.Panicln("Failed while binding skynet to", vHost)
					}
					go http.Serve(skyL, r)
					log.Println("Serving SHTTP for", vHost)
				}
			}
		}
	}

	for _, sh := range sysHost {
		hs[sh] = http.DefaultServeMux
	}

	if srvErr := skynet.ListenAndServe("tcp4", "0.0.0.0:"+skyPort); srvErr != nil {
		log.Panicln(srvErr)
	}

	if httpServeErr := http.ListenAndServe(httpBind, hs); httpServeErr != nil {
		log.Panic(httpServeErr)
	}
}
