package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"rheincni"
	"rheincni/ipam"
	"rheincni/ipam/gen/ipamv1"
	sqlite3 "rheincni/thirdparty/sqlite/bindings"
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
	database, err := sqlite3.Open("/var/lib/rheincni/ipam.db")
	if err != nil {
		log.Fatalf("failed to open IPAM database: %v", err)
	}
	defer database.Close()
	service, err := ipam.NewIPAMService(database, rheincni.IPSubnet{IP: 0, Prefix: 8})
	if err != nil {
		log.Fatalf("failed to initialize IPAM service: %v", err)
	}
	ipamv1.RegisterIPAMServiceServer(grpcServer, service)

	go func() {
		err = grpcServer.Serve(lis)
		if err != nil {
			panic(err.Error())
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	grpcServer.GracefulStop()
}
