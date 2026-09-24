package ipam

import (
	"context"
	"fmt"
	"rheincni/ipam/gen/ipamv1"
	sqlite3 "rheincni/thirdparty/sqlite/bindings"

	"google.golang.org/protobuf/types/known/emptypb"
)

type IPAMService struct {
	DB *sqlite3.DB
	ipamv1.IPAMServiceServer
}

func (s *IPAMService) AllocateIP(context.Context, r *ipamv1.AllocateIPRequest) (*ipamv1.IP, error) {
	r.HostEthIndex
	fmt.Println("allocate the ip")

}
func (s *IPAMService) DeallocateIP(context.Context, r *ipamv1.IP) (*emptypb.Empty, error) {
	fmt.Println("deallocate the ip")
	return &emptypb.Empty{}, nil
}
