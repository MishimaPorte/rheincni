package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"rheincni/ipam/gen/ipamv1"
	"syscall"

	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
	thing.RegisterMyServiceServer(grpcServer, new(ThingService{}))

	ipamv1.RegisterIPAMServiceServer(grpcServer, opts)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		os.Exit(0)
	}()

	select {}
}
