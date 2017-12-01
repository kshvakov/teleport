package teleport

import (
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"sort"
	"sync/atomic"
	"time"
)

func NewServer() *Server {
	server := Server{
		logf:     func(string, ...interface{}) {},
		version:  version1,
		hostname: hostname,
		unixtime: time.Now().Unix(),
	}
	go server.ticker()
	return &server
}

type Server struct {
	logf           func(string, ...interface{})
	version        uint16
	hostname       string
	handlers       []*handler
	unixtime       int64
	healthCheckers []HealthChecker
}

// SetDebug enable debug mode
func (server *Server) SetDebug() {
	server.logf = log.New(os.Stdout, "[teleport][server]", 0).Printf
}

// Register handler in the server
func (server *Server) Register(v interface{}) error {
	return server.RegisterName(reflect.Indirect(reflect.ValueOf(v)).Type().Name(), v)
}

// RegisterName handler with specific name in the server
func (server *Server) RegisterName(name string, v interface{}) error {
	if len(name) == 0 {
		return fmt.Errorf("name of handler can not be empty")
	}
	switch value := reflect.ValueOf(v); value.Type().Kind() {
	case reflect.Func:
		return server.addHandler(name, value)
	default:
		switch v := v.(type) {
		case HealthChecker:
			server.logf("add health checker for '%s'", name)
			server.healthCheckers = append(server.healthCheckers, v)
		}
		typeOf := value.Type()
		for i := 0; i < value.NumMethod(); i++ {
			var method = typeOf.Method(i)
			if len(method.PkgPath) != 0 {
				continue
			}
			if err := server.addHandler(name+"."+method.Name, value.Method(i)); err != nil {
				return err
			}
		}
		return nil
	}
}

func (server *Server) Serve(l net.Listener) {
	defer l.Close()
	for {
		if conn, err := l.Accept(); err == nil {
			go func() {
				if err := sessionStart(server, conn); err != nil {
					server.logf("handle err: %v", err)
				}
				conn.Close()
			}()
		}
	}
}

func (server *Server) addHandler(name string, v reflect.Value) error {
	if _, found := server.handler(name); found {
		return fmt.Errorf("handler '%s' already exists", name)
	}
	var (
		fn     = v.Type()
		numIn  = fn.NumIn()
		numOut = fn.NumOut()
	)
	if fn.Kind() == reflect.Ptr {
		fn = fn.Elem()
	}
	switch {
	case numOut != 1 || numIn != 2:
		return nil
	case !fn.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()):
		return nil
	case !fn.In(0).Implements(reflect.TypeOf((*Context)(nil)).Elem()):
	case !fn.In(1).Implements(reflect.TypeOf((*Args)(nil)).Elem()):
		return nil
	}
	args := fn.In(1)
	if args.Kind() == reflect.Ptr {
		args = args.Elem()
	}
	server.logf("add handler '%s', numOut=%d", name, numOut)
	server.handlers = append(server.handlers, &handler{
		fn:   v,
		name: name,
		args: reflect.New(args).Interface().(Args),
	})
	sort.Slice(server.handlers, func(a, b int) bool {
		return server.handlers[a].name < server.handlers[b].name
	})
	return nil
}

func (server *Server) handler(name string) (*handler, bool) {
	i := sort.Search(len(server.handlers), func(i int) bool { return server.handlers[i].name >= name })
	if i < len(server.handlers) && server.handlers[i].name == name {
		return server.handlers[i], true
	}
	return nil, false
}

func (server *Server) ticker() {
	for tick := time.Tick(time.Second); ; {
		select {
		case <-tick:
			atomic.AddInt64(&server.unixtime, int64(time.Second))
		}
	}
}

func (server *Server) now() time.Time {
	return time.Unix(atomic.LoadInt64(&server.unixtime), 0)
}
