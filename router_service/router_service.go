package routerservice

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"rheincni"
	rsv1 "rheincni/router_service/gen/routerservicev1"

	"github.com/MishimaPorte/ebpf-loader/loader"
	"google.golang.org/protobuf/types/known/emptypb"

	// #include "../lib/types.h"
	// #include <linux/bpf.h>
	// #include <bpf/bpf_helpers.h>
	"C"
)

type RouterService struct {
	EndpointMap rheincni.MapFd
	ProgramFd   loader.ProgramFd
	EthToLink   map[string]loader.LinkFd
	rsv1.UnimplementedRouterServiceServer
}

func (r *RouterService) GetInfo(ctx context.Context, in *emptypb.Empty) (*rsv1.RouterInfo, error) {
	return new(rsv1.RouterInfo), nil
}

func (r *RouterService) AddRoute(ctx context.Context, in *rsv1.AddRouteReq) (*emptypb.Empty, error) {
	err := rheincni.UpdateEndpoint(r.EndpointMap, in.TargetIp, in.TargetMac, int(in.TargetIfindex))
	if err != nil {
		return nil, fmt.Errorf("failed to update routes map: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (r *RouterService) RemoveRoute(ctx context.Context, in *rsv1.RemoveRouteReq) (*emptypb.Empty, error) {
	err := rheincni.DeleteEndpoint(r.EndpointMap, in.TargetIp)
	if err != nil {
		return nil, fmt.Errorf("failed to delete a route: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (r *RouterService) AddVethPair(ctx context.Context, in *rsv1.AddVethPairReq) (*rsv1.AddVethPairResp, error) {
	var mp rheincni.MacPair // first is host mac, second is peer mac
	rheincni.GenerateRandomMacPair(&mp)
	err := rheincni.CreateVethPeer(in.HostEthName, in.PeerEthName, in.Netns, mp)
	if err != nil {
		return nil, fmt.Errorf("create veth peer veth: %w", err)
	}
	linkFd, err := loader.AttachProgramToInterface(r.ProgramFd, in.HostEthName)
	if err != nil {
		return nil, err
	}
	r.EthToLink[in.HostEthName] = linkFd
	hostVeth, err := net.InterfaceByName(in.HostEthName)
	if err != nil {
		return nil, fmt.Errorf("find host veth %s: %w", in.HostEthName, err)
	}
	return &rsv1.AddVethPairResp{
		HostMac: binary.BigEndian.Uint64(mp[0][:]),
		PeerMac: binary.BigEndian.Uint64(mp[1][:]),
		HostIdx: uint32(hostVeth.Index),
	}, nil
}

func NewRouterService(routerProgramElf []byte, routerMapName string) (*RouterService, error) {
	mapfd, err := rheincni.CreateEndpointMap()
	if err != nil {
		return nil, err
	}

	programFd, err := loader.LoadProgram("GPL", routerProgramElf,
		loader.OverrideMap(routerMapName, int(mapfd)))
	if err != nil {
		return nil, err
	}

	return &RouterService{
		ProgramFd:   programFd,
		EndpointMap: mapfd,
		EthToLink:   make(map[string]loader.LinkFd, 256),
	}, nil
}
