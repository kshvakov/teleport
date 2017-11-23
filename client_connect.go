package teleport

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type connect struct {
	net.Conn
	encoder  encoder
	decoder  decoder
	client   *Client
	lifetime time.Time
	server   ServerInfo
	closed   int32
}

func (conn *connect) handshake() (err error) {
	if conn.server.Version != 0 {
		return nil
	}
	if err = conn.encoder.uint8(Hello); err != nil {
		return err
	}
	if err = conn.encoder.uint16(conn.client.version); err != nil {
		return err
	}
	if err = conn.encoder.string(conn.client.hostname); err != nil {
		return err
	}
	switch packet, err := conn.decoder.uint8(); true {
	case err != nil:
		return err
	case packet == Exception:
		return conn.exception()
	case packet == Hello:
		if conn.server.Version, err = conn.decoder.uint16(); err != nil {
			return err
		}
		if conn.server.Hostname, err = conn.decoder.string(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unexpected packet")
	}
	return nil
}

func (conn *connect) exception() (err error) {
	var e Error
	if e.message, err = conn.decoder.string(); err != nil {
		return err
	}
	return &e
}

func (conn *connect) release() {
	conn.client.logf("release connect: %s -> %s", conn.LocalAddr(), conn.RemoteAddr())
	if atomic.LoadInt32(&conn.closed) == 0 && conn.lifetime.After(time.Now()) && len(conn.client.idleConns) < conn.client.maxIdleConns {
		conn.client.idleConns <- conn
	} else {
		conn.close()
	}
	<-conn.client.openConns
}

func (conn *connect) close() {
	if atomic.CompareAndSwapInt32(&conn.closed, 0, 1) {
		conn.client.logf("close connect: %s -> %s", conn.LocalAddr(), conn.RemoteAddr())
		conn.Conn.Close()
	}
}
