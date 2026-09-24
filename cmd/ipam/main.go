package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"rheincni/ipam"
	"rheincni/ipam/gen/ipamv1"
	sqlite3 "rheincni/thirdparty/sqlite/bindings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("failed to configure Kubernetes client: %v", err)
	}
	client, err := coreclient.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create Kubernetes client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	subnet, err := waitForNodePodCIDR(ctx, client.Nodes(), os.Getenv("NODE_NAME"))
	cancel()
	if err != nil {
		log.Fatalf("failed to get node PodCIDR: %v", err)
	}
	log.Printf("using node PodCIDR %s", subnet)

	database, err := sqlite3.Open("/var/lib/rheincni/ipam.db")
	if err != nil {
		log.Fatalf("failed to open IPAM database: %v", err)
	}
	defer database.Close()
	service, err := ipam.NewIPAMService(database, subnet)
	if err != nil {
		log.Fatalf("failed to initialize IPAM service: %v", err)
	}

	lis, err := net.Listen("tcp", "localhost:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)
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
