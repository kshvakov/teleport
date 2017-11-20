package teleport

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func Test_T(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		server := NewServer()
		server.SetDebug()
		server.Serve(listener)
	}()
	client := NewClient(listener.Addr().String(), nil)
	client.SetDebug()

	t.Log(client.Stat())
	t.Log(client.connect())
	t.Log(client.Stat())
	conn, err := client.connect()
	t.Log(conn, err)
	if err == nil {
		conn.release()
	}
	t.Log(client.Stat())
	t.Log(client.connect())

	t.Log(client.Stat())
	t.Log(client.connect())

	t.Log(client.Stat())
	t.Log(client.connect())
}

func Test_ClientCall(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		server := NewServer()
		server.SetDebug()
		server.RegisterName("fn", func(ctx Context, args *strArgs) error {
			return ctx.WriteResponse("Hello, " + string(*args))
		})
		server.Serve(listener)
	}()
	client := NewClient(listener.Addr().String(), nil)
	client.SetDebug()
	var result string
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()
	for i := 0; i < 10; i++ {
		t.Log(client.DoContext(ctx, "fn", strArgs(fmt.Sprintf("Test client (%d).", i)), &result), result)
	}
}

func Benchmark_Client(b *testing.B) {
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		b.Fatal(err)
	}
	go func() {
		server := NewServer()
		server.RegisterName("fn", func(ctx Context, args *strArgs) error {
			return ctx.WriteResponse("Hello, " + string(*args))
		})
		server.Serve(listener)
	}()
	client := NewClient(listener.Addr().String(), &Options{
		MaxOpenConns: 400,
		MaxIdleConns: 350,
	})

	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(25)

	b.RunParallel(func(pb *testing.PB) {
		var result string
		for pb.Next() {
			if err = client.Do("fn", strArgs("Client"), &result); err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(result)))
		}
	})
}

func Benchmark_ClientWithTimeout(b *testing.B) {
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		b.Fatal(err)
	}
	go func() {
		server := NewServer()
		server.RegisterName("fn", func(ctx Context, args *strArgs) error {
			return ctx.WriteResponse("Hello, " + string(*args))
		})
		server.Serve(listener)
	}()
	client := NewClient(listener.Addr().String(), &Options{
		MaxOpenConns: 400,
		MaxIdleConns: 350,
	})

	b.ResetTimer()
	b.ReportAllocs()
	b.SetParallelism(25)

	b.RunParallel(func(pb *testing.PB) {
		var result string
		for pb.Next() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*120)
			if err = client.DoContext(ctx, "fn", strArgs("Client"), &result); err != nil {
				b.Fatal(err)
			}
			cancel()
			b.SetBytes(int64(len(result)))
		}
	})
}
