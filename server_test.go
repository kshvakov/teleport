package teleport

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Handshake(t *testing.T) {
	server := NewServer()
	server.SetDebug()
	server.hostname = "test_server"
	if listener, err := net.Listen("tcp", "127.0.0.1:"); assert.NoError(t, err) {
		go server.Serve(listener)
		if conn, err := net.Dial("tcp", listener.Addr().String()); assert.NoError(t, err) {
			defer conn.Close()
			var (
				decoder = decoder{
					input: conn,
				}
				encoder = encoder{
					output: conn,
				}
			)
			if err := encoder.uint8(Hello); assert.NoError(t, err) {
				require.NoError(t, encoder.uint16(version1))
				require.NoError(t, encoder.string("test_client"))
				if packet, err := decoder.uint8(); assert.NoError(t, err) && assert.Equal(t, Hello, packet) {
					if version, err := decoder.uint16(); assert.NoError(t, err) {
						assert.Equal(t, server.version, version)
					}
					if hostname, err := decoder.string(); assert.NoError(t, err) {
						assert.Equal(t, server.hostname, hostname)
					}
				}
			}
		}
	}
}

func Test_HandshakeException(t *testing.T) {
	server := NewServer()
	server.SetDebug()
	server.hostname = "test_server"
	if listener, err := net.Listen("tcp", "127.0.0.1:"); assert.NoError(t, err) {
		go server.Serve(listener)
		if conn, err := net.Dial("tcp", listener.Addr().String()); assert.NoError(t, err) {
			defer conn.Close()
			var (
				decoder = decoder{
					input: conn,
				}
				encoder = encoder{
					output: conn,
				}
			)
			if err := encoder.uint8(Ping); assert.NoError(t, err) {
				require.NoError(t, encoder.uint16(version1))
				require.NoError(t, encoder.string("test_client"))
				if packet, err := decoder.uint8(); assert.NoError(t, err) && assert.Equal(t, Exception, packet) {
					if hostname, err := decoder.string(); assert.NoError(t, err) {
						assert.Contains(t, hostname, "unexpected packet")
					}
				}
			}
		}
	}
}

func Test_PingPong(t *testing.T) {
	server := NewServer()
	server.SetDebug()
	server.hostname = "test_server"
	if listener, err := net.Listen("tcp", "127.0.0.1:"); assert.NoError(t, err) {
		go server.Serve(listener)
		if conn, err := net.Dial("tcp", listener.Addr().String()); assert.NoError(t, err) {
			defer conn.Close()
			var (
				decoder = decoder{
					input: conn,
				}
				encoder = encoder{
					output: conn,
				}
			)
			require.NoError(t, testHandshake(&decoder, &encoder))
			for i := 0; i < 3; i++ {
				if err := encoder.uint8(Ping); assert.NoError(t, err) {
					if value, err := decoder.uint8(); assert.NoError(t, err) {
						assert.Equal(t, Pong, value)
					}
				}
			}
		}
	}
}

type testHealthCheck struct{ err error }

func (t *testHealthCheck) HealthCheck() error { return t.err }
func Test_HealthCheckOK(t *testing.T) {
	server := NewServer()
	server.SetDebug()
	server.Register(&testHealthCheck{})
	if listener, err := net.Listen("tcp", "127.0.0.1:"); assert.NoError(t, err) {
		go server.Serve(listener)
		if conn, err := net.Dial("tcp", listener.Addr().String()); assert.NoError(t, err) {
			defer conn.Close()
			var (
				decoder = decoder{
					input: conn,
				}
				encoder = encoder{
					output: conn,
				}
			)
			require.NoError(t, testHandshake(&decoder, &encoder))
			if err := encoder.uint8(HealthCheck); assert.NoError(t, err) {
				if value, err := decoder.uint8(); assert.NoError(t, err) {
					assert.Equal(t, HealthCheck, value)
				}
			}
		}
	}
}

func Test_HealthCheckError(t *testing.T) {
	server := NewServer()
	server.SetDebug()
	server.Register(&testHealthCheck{err: fmt.Errorf("health check with error")})
	if listener, err := net.Listen("tcp", "127.0.0.1:"); assert.NoError(t, err) {
		go server.Serve(listener)
		if conn, err := net.Dial("tcp", listener.Addr().String()); assert.NoError(t, err) {
			defer conn.Close()
			var (
				decoder = decoder{
					input: conn,
				}
				encoder = encoder{
					output: conn,
				}
			)
			require.NoError(t, testHandshake(&decoder, &encoder))
			if err := encoder.uint8(HealthCheck); assert.NoError(t, err) {
				if value, err := decoder.uint8(); assert.NoError(t, err) {
					if assert.Equal(t, Exception, value) {
						if value, err := decoder.string(); assert.NoError(t, err) {
							assert.Contains(t, value, "health check with error")
						}
					}
				}
			}
		}
	}
}

type strArgs string

func (strArgs) Validate() error { return nil }

type sumArgs struct {
	A, B int
}

func (sumArgs) Validate() error { return nil }

type TestStruct struct{}

func (TestStruct) Sum(ctx Context, args *sumArgs) error {
	return ctx.WriteResponse(args.A + args.B)
}

func Test_Call(t *testing.T) {
	server := NewServer()
	server.SetDebug()
	server.Register(&TestStruct{})
	server.RegisterName("fn", func(ctx Context, args *strArgs) error {
		return ctx.WriteResponse("Hello, " + string(*args))
	})
	if listener, err := net.Listen("tcp", "127.0.0.1:"); assert.NoError(t, err) {
		go server.Serve(listener)
		if conn, err := net.Dial("tcp", listener.Addr().String()); assert.NoError(t, err) {
			defer conn.Close()
			var (
				decoder = decoder{
					input: conn,
				}
				encoder = encoder{
					output: conn,
				}
			)
			require.NoError(t, testHandshake(&decoder, &encoder))
			{
				require.NoError(t, encoder.uint8(Call))
				require.NoError(t, encoder.string("fn"))
				require.NoError(t, encoder.args(strArgs("Test")))
				require.NoError(t, encoder.int64(0))
				if packet, err := decoder.uint8(); assert.NoError(t, err) && assert.Equal(t, Data, packet) {
					var result string
					if err := decoder.result(&result); assert.NoError(t, err) {
						assert.Equal(t, "Hello, Test", result)
					}
				}
			}
			{
				require.NoError(t, encoder.uint8(Call))
				require.NoError(t, encoder.string("TestStruct.Sum"))
				require.NoError(t, encoder.args(&sumArgs{A: 2, B: 3}))
				require.NoError(t, encoder.int64(0))
				if packet, err := decoder.uint8(); assert.NoError(t, err) && assert.Equal(t, Data, packet) {
					var result int
					if err := decoder.result(&result); assert.NoError(t, err) {
						assert.Equal(t, int(5), result)
					}
				}
			}
		}
	}
}

//
func testHandshake(decoder *decoder, encoder *encoder) error {
	if err := encoder.uint8(Hello); err != nil {
		return err
	}
	if err := encoder.uint16(version1); err != nil {
		return err
	}
	if err := encoder.string("test_client"); err != nil {
		return err
	}
	packet, err := decoder.uint8()
	if err != nil {
		return err
	}
	if packet != Hello {
		return fmt.Errorf("unexpected packet: %d", packet)
	}
	if _, err = decoder.uint16(); err != nil {
		return err
	}
	if _, err = decoder.string(); err != nil {
		return err
	}
	return nil
}
