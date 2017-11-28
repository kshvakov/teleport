package teleport

import (
	"context"
	"net"
	"sync/atomic"
	"time"
)

var _ Context = &serverContext{}

func newContext(session *session, deadline int64) *serverContext {
	ctx := serverContext{
		err:      context.DeadlineExceeded,
		conn:     session.conn,
		encoder:  &session.encoder,
		deadline: deadline,
	}
	if deadline != 0 {
		ctx.done = make(chan struct{}, 1)
		go ctx.watch()
	}
	return &ctx
}

type serverContext struct {
	err      error
	conn     net.Conn
	done     chan struct{}
	encoder  *encoder
	deadline int64
	finished int32
	closed   int32
}

func (ctx *serverContext) Err() error {
	return ctx.err
}

func (ctx *serverContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *serverContext) Deadline() (deadline time.Time, ok bool) {
	if ctx.deadline != 0 {
		return time.Now().Add(time.Duration(ctx.deadline)), true
	}
	return time.Time{}, false
}

func (ctx *serverContext) Value(key interface{}) interface{} {
	return nil
}

func (ctx *serverContext) WriteResponse(v interface{}) error {
	if atomic.CompareAndSwapInt32(&ctx.finished, 0, 1) {
		return ctx.encoder.result(v)
	}
	return nil
}

func (ctx *serverContext) SetReadDeadline(t time.Time) error {
	return ctx.conn.SetReadDeadline(t)
}

func (ctx *serverContext) SetWriteDeadline(t time.Time) error {
	return ctx.conn.SetWriteDeadline(t)
}

func (ctx *serverContext) watch() {
	select {
	case <-ctx.done:
		ctx.close()
	}
}

func (ctx *serverContext) close() {
	if atomic.CompareAndSwapInt32(&ctx.closed, 0, 1) {
		ctx.err = nil
		if ctx.done != nil {
			close(ctx.done)
		}
	}
}
