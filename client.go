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

func NewClient(addr string) *Client {
	return &Client{
		dial:            defaultDialer(addr),
		logf:            func(string, ...interface{}) {},
		idleConn:        make(chan *connect, 350),
		openConn:        make(chan struct{}, 400),
		maxRetry:        2,
		maxIdleConn:     200,
		openConnTimeout: time.Second,
	}
}

type Client struct {
	logf            func(string, ...interface{})
	mutex           sync.RWMutex
	dial            func() (net.Conn, error)
	version         uint16
	hostname        string
	idleConn        chan *connect
	openConn        chan struct{}
	openConnTimeout time.Duration
	maxRetry        int
	maxIdleConn     int
}

func (client *Client) SetDebug() {
	client.logf = log.New(os.Stdout, "[teleport][client]", 0).Printf
}

func (client *Client) Stat() ClientStat {
	return ClientStat{
		NumOpenConn: len(client.openConn),
		NumIdleConn: len(client.idleConn),
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
		timeout  = time.Tick(client.openConnTimeout)
	)
	for {
		select {
		case client.openConn <- struct{}{}:
			select {
			case conn := <-client.idleConn:
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
					deadline: time.Now().Add(time.Minute),
				}, nil
			case attempts >= client.maxRetry:
				<-client.openConn
				return nil, err
			default:
				<-client.openConn
				attempts++
			}
		case <-timeout:
			return nil, ErrOpenOrReuseConnTimeout
		case <-tick:
		}
	}
}
