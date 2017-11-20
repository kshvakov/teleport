package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/kshvakov/teleport"
)

type strArgs string

func (strArgs) Validate() error { return nil }

type structArgs struct {
	A, B int
}

func (args *structArgs) Validate() error {
	if args.A == 0 || args.B == 0 {
		return fmt.Errorf("A or B can not be empty")
	}
	return nil
}

var (
	h, s int64
)

type Service struct {
}

func (*Service) Hello(ctx teleport.Context, args *strArgs) error {
	atomic.AddInt64(&h, 1)
	return ctx.WriteResponse(fmt.Sprintf("Hi, %s", *args))
}

func (*Service) Sum(ctx teleport.Context, args *structArgs) error {
	atomic.AddInt64(&s, 1)
	sum := args.A + args.B
	if sum > 1000 {
		return fmt.Errorf("SUM could not be less than 1000 (%d)", sum)
	}
	return ctx.WriteResponse(sum)
}

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		server := teleport.NewServer()
		server.Register(&Service{})
		server.Serve(listener)
	}()
	client := teleport.NewClient(listener.Addr().String())

	for i := 0; i < 58; i++ {
		go hello(i, client)
	}
	for i := 0; i < 58; i++ {
		go sum(client)
	}

	for {
		var sum int
		time.Sleep(time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := client.DoContext(ctx, "Service.Sum", &structArgs{A: 1000, B: 25}, sum); err != nil {
			log.Println(err)
		}
		cancel()
	}
}

func hello(v int, client *teleport.Client) {
	var result string
	var i int
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := client.DoContext(ctx, "Service.Hello", strArgs(fmt.Sprintf("Client(%d) %d", v, i)), &result); err != nil {
			log.Println(err)
		}
		cancel()
		i++
		if (i % 15000) == 0 {
			log.Println(result, atomic.LoadInt64(&h), atomic.LoadInt64(&s))
		}
	}
}

func sum(client *teleport.Client) {
	var result int
	var i int
	for {
		i++
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := client.DoContext(ctx, "Service.Sum", &structArgs{A: 987 % i, B: 25 % i}, &result); err != nil {
			if (i % 1000) == 0 {
				log.Println(err)
			}
		}
		cancel()
		if (i % 15000) == 0 {
			log.Println("SUM", result, atomic.LoadInt64(&h), atomic.LoadInt64(&s))
		}
	}
}
