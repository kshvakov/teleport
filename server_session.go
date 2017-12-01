package teleport

import (
	"bufio"
	"fmt"
	"io"
	"net"
)

func sessionStart(server *Server, conn net.Conn) error {
	session := session{
		conn:   conn,
		server: server,
		encoder: encoder{
			output: conn,
		},
		decoder: decoder{
			input: bufio.NewReader(conn),
		},
	}
	return session.start()
}

type session struct {
	conn       net.Conn
	server     *Server
	encoder    encoder
	decoder    decoder
	clientInfo ClientInfo
}

func (session *session) start() error {
	if err := session.handshake(); err != nil {
		return session.exception(fmt.Errorf("handshake: %v", err))
	}
	var (
		err    error
		packet uint8
	)
	for {
		//	session.conn.SetDeadline()
		if packet, err = session.decoder.uint8(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		switch packet {
		case Call:
			if err := session.call(); err != nil {
				return session.exception(fmt.Errorf("call: %v", err))
			}
		case Ping:
			session.server.logf("<- ping")
			if err := session.ping(); err != nil {
				return session.exception(fmt.Errorf("ping: %v", err))
			}
			session.server.logf("-> pong")
		case HealthCheck:
			session.server.logf("<- health check")
			if err := session.healthCheck(); err != nil {
				session.server.logf("-> health check: %v", err)
				return session.exception(fmt.Errorf("health check: %v", err))
			}
			session.server.logf("-> health check: ok")
		case Cancel:
			session.server.logf("<- cancel")
			return nil
		default:
			return fmt.Errorf("session: unexpected packet '%d'", packet)
		}
	}
}

func (session *session) ping() error {
	return session.encoder.uint8(Pong)
}

func (session *session) handshake() (err error) {
	switch packet, err := session.decoder.uint8(); true {
	case packet != Hello:
		return fmt.Errorf("unexpected packet '%d'", packet)
	case err != nil:
		return err
	}
	{ // read client info
		if session.clientInfo.Version, err = session.decoder.uint16(); err != nil {
			return err
		}
		if session.clientInfo.Hostname, err = session.decoder.string(); err != nil {
			return err
		}
	}
	session.server.logf("handshake <- version=%d, hostname=%s", session.clientInfo.Version, session.clientInfo.Hostname)
	{ // write server info
		if err = session.encoder.uint8(Hello); err != nil {
			return err
		}
		if err = session.encoder.uint16(session.server.version); err != nil {
			return err
		}
	}
	session.server.logf("handshake -> version=%d, hostname=%s", session.server.version, session.server.hostname)
	return session.encoder.string(session.server.hostname)
}

func (session *session) healthCheck() error {
	for _, h := range session.server.healthCheckers {
		if err := h.HealthCheck(); err != nil {
			return err
		}
	}
	return session.encoder.uint8(HealthCheck)
}

func (session *session) call() error {
	var (
		args        Args
		deadline    int64
		method, err = session.decoder.string()
	)
	if err != nil {
		return err
	}

	handler, found := session.server.handler(method)
	if !found {
		return session.exception(fmt.Errorf("method '%s' not found", method))
	}
	if args, err = session.decoder.args(handler.args); err != nil {
		return session.exception(err)
	}
	if deadline, err = session.decoder.int64(); err != nil {
		return session.exception(err)
	}
	if err := args.Validate(); err != nil {
		return session.exception(err)
	}

	session.server.logf("<- call method=%s, args=%#v, deadline=%d", method, args, deadline)

	ctx := newContext(session, deadline)
	defer ctx.close()
	if err := handler.call(ctx, args); err != nil {
		return session.exception(err)
	}
	return ctx.WriteResponse(nil)
}

func (session *session) exception(err error) error {
	if err := session.encoder.uint8(Exception); err != nil {
		return err
	}
	return session.encoder.string(err.Error())
}
