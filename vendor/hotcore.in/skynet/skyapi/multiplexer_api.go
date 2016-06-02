package skyapi

import (
	"io"
	"net"
	"time"
)

type Multiplexer interface {
	DialTimeout(network string, hp string, t time.Duration) (net.Conn, error)
	Dial(network string, hp string) (net.Conn, error)
	Bind(network string, hp string) (net.Listener, error)
	Attach(conn io.ReadWriter)
	ListenAndServe(network, address string) error
	Join(network, address string) error
	Services() []ServiceId
	Routes() []ServiceId

	WithEnv(env ...string) Multiplexer
	WithLoopBack() Multiplexer
	Client() Multiplexer
	Server() Multiplexer
	New() Multiplexer

	HttpDial(net, host string) (net.Conn, error)
}
