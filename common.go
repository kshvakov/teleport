package teleport

import (
	"os"
	"time"
)

var hostname, _ = os.Hostname()

const (
	version1 uint16 = iota + 1
	currentVersion
)

const (
	Hello       uint8 = 1
	Ping        uint8 = 2
	Pong        uint8 = 3
	Call        uint8 = 4
	Data        uint8 = 5
	Cancel      uint8 = 6
	Exception   uint8 = 7
	HealthCheck uint8 = 8
)

type ServerInfo struct {
	Version  uint16
	Hostname string
}

type ClientInfo struct {
	Version  uint16
	Hostname string
}

type ClientStat struct {
	NumOpenConn int
	NumIdleConn int
}

type Error struct {
	message string
}

func (e *Error) Error() string {
	return e.message
}

type Args interface {
	Validate() error
}

type Context interface {
	Err() error
	Done() <-chan struct{}
	Deadline() (deadline time.Time, ok bool)
	Value(key interface{}) interface{}
	WriteResponse(interface{}) error
}

type HealthChecker interface {
	HealthCheck() error
}
