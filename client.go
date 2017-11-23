package teleport

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

var defaultDialer = func(addr string) func() (net.Conn, error) {
	return func() (net.Conn, error) {
		return net.DialTimeout("tcp", addr, 50*time.Millisecond)
	}
}

// Options client
type Options struct {
	Dial            func() (net.Conn, error)
	MaxRetry        int
	MaxOpenConns    int
	MaxIdleConns    int
	ConnTimeout     time.Duration
	ConnDeadline    time.Duration
	ConnMaxLifetime time.Duration
}

const (
	DefaultMaxRetry        = 2
	DefaultMaxOpenConns    = 50
	DefaultMaxIdleConns    = 25
	DefaultConnTimeout     = 50 * time.Millisecond
	DefaultConnDeadline    = 5 * time.Second
	DefaultConnMaxLifetime = time.Hour
)

// NewClient init client with options
func NewClient(addr string, options *Options) *Client {
	var (
		dial            = defaultDialer(addr)
		maxRetry        = DefaultMaxRetry
		maxOpenConns    = DefaultMaxOpenConns
		maxIdleConns    = DefaultMaxIdleConns
		connTimeout     = DefaultConnTimeout
		connDeadline    = DefaultConnDeadline
		connMaxLifetime = DefaultConnMaxLifetime
	)
	if options != nil {
		if options.MaxRetry != 0 {
			maxRetry = options.MaxRetry
		}
		if options.MaxIdleConns != 0 {
			maxOpenConns = options.MaxOpenConns
		}
		if options.MaxOpenConns != 0 {
			maxIdleConns = options.MaxIdleConns
		}
		if options.ConnTimeout != 0 {
			connTimeout = options.ConnTimeout
		}
		if options.ConnDeadline != 0 {
			connDeadline = options.ConnDeadline
		}
		if options.Dial != nil {
			dial = options.Dial
		}
	}
	return &Client{
		dial:            dial,
		logf:            func(string, ...interface{}) {},
		idleConns:       make(chan *connect, maxIdleConns),
		openConns:       make(chan struct{}, maxOpenConns),
		maxRetry:        maxRetry,
		connTimeout:     connTimeout,
		connDeadline:    connDeadline,
		connMaxLifetime: connMaxLifetime,
		maxIdleConns:    maxIdleConns,
	}
}

// Client RPC-client
type Client struct {
	logf            func(string, ...interface{})
	mutex           sync.RWMutex
	dial            func() (net.Conn, error)
	version         uint16
	hostname        string
	idleConns       chan *connect
	openConns       chan struct{}
	connTimeout     time.Duration
	connDeadline    time.Duration
	connMaxLifetime time.Duration
	maxRetry        int
	maxIdleConns    int
}

func (client *Client) SetDebug() {
	client.logf = log.New(os.Stdout, "[teleport][client]", 0).Printf
}

func (client *Client) Stat() ClientStat {
	return ClientStat{
		NumOpenConns: len(client.openConns),
		NumIdleConns: len(client.idleConns),
	}
}

func (client *Client) Do(method string, args Args, result interface{}) error {
	return client.do(context.Background(), method, args, result)
}

func (client *Client) DoContext(ctx context.Context, method string, args Args, result interface{}) error {
	return client.do(ctx, method, args, result)
}

func (client *Client) do(ctx context.Context, method string, args Args, result interface{}) error {
	if err := args.Validate(); err != nil {
		return err
	}
	connect, err := client.connect()
	if err != nil {
		return err
	}
	defer connect.release()
	if finish := client.watch(ctx, connect); finish != nil {
		defer finish()
	}
	if err := connect.encoder.uint8(Call); err != nil {
		return err
	}
	if err := connect.encoder.string(method); err != nil {
		return err
	}
	if err := connect.encoder.args(args); err != nil {
		return err
	}
	switch deadline, ok := ctx.Deadline(); true {
	case ok:
		connect.encoder.int64(int64(deadline.Sub(time.Now())))
	default:
		connect.encoder.int64(0)
	}
	switch packet, err := connect.decoder.uint8(); packet {
	case Data:
		if err := connect.decoder.result(result); err != nil {
			return err
		}
		return nil
	case Exception:
		return connect.exception()
	default:
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected packet: %d", packet)
	}
}

func (client *Client) watch(ctx context.Context, conn *connect) func() {
	if done := ctx.Done(); done != nil {
		finished := make(chan struct{})
		go func() {
			select {
			case <-done:
				conn.close()
				finished <- struct{}{}
			case <-finished:
			}
		}()
		return func() {
			select {
			case <-finished:
			case finished <- struct{}{}:
			}
		}
	}
	return nil
}

func (client *Client) connect() (conn *connect, err error) {
	for i := 0; i < 2; i++ {
		if conn, err = client.openOrReuseConn(); err == nil {
			if err = conn.handshake(); err == nil {
				return conn, nil
			}
			conn.close()
		}
	}
	return nil, err
}

var ErrOpenOrReuseConnTimeout = errors.New("Open connect timeout")

func (client *Client) openOrReuseConn() (*connect, error) {
	var (
		attempts int
		tick     = time.Tick(time.Millisecond)
		timeout  = time.Tick(client.connTimeout)
	)
	for {
		select {
		case client.openConns <- struct{}{}:
			select {
			case conn := <-client.idleConns:
				client.logf("reuse connect: %s -> %s", conn.LocalAddr(), conn.RemoteAddr())
				return conn, nil
			default:
			}
			switch conn, err := client.dial(); true {
			case err == nil:
				client.logf("open new connect: %s -> %s", conn.LocalAddr(), conn.RemoteAddr())
				return &connect{
					Conn: conn,
					encoder: encoder{
						output: conn,
					},
					decoder: decoder{
						input: bufio.NewReader(conn),
					},
					client:   client,
					lifetime: time.Now().Add(client.connMaxLifetime),
				}, nil
			case attempts >= client.maxRetry:
				<-client.openConns
				return nil, err
			default:
				<-client.openConns
				attempts++
			}
		case <-timeout:
			return nil, ErrOpenOrReuseConnTimeout
		case <-tick:
		}
	}
}
