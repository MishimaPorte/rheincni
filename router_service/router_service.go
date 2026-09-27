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
import (
	"errors"
	"os"
)

const BpfPinningBasePath = "/sys/fs/bpf/rheindaemon"
const BpfPinningEndpointMap = BpfPinningBasePath + "/maps/endpoints"
const BpfPinningRouterProgram = BpfPinningBasePath + "/programs/router"

type RouterService struct {
	EndpointMap rheincni.MapFd
	ProgramFd   loader.ProgramFd
	rsv1.UnimplementedRouterServiceServer
}

func (r *RouterService) GetInfo(ctx context.Context, in *emptypb.Empty) (*rsv1.RouterInfo, error) {
	return &rsv1.RouterInfo{
		NumberOfRoutes: 0,
	}, nil
}

func (r *RouterService) AddRoute(ctx context.Context, in *rsv1.AddRouteReq) (*emptypb.Empty, error) {
	var key [4]byte
	binary.BigEndian.PutUint32(key[:], in.TargetIp)
	err := rheincni.UpdateEndpoint(r.EndpointMap, key, in.TargetMac, int(in.TargetIfindex))
	if err != nil {
		return nil, fmt.Errorf("failed to update routes map: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (r *RouterService) RemoveRoute(ctx context.Context, in *rsv1.RemoveRouteReq) (*emptypb.Empty, error) {
	var key [4]byte
	binary.BigEndian.PutUint32(key[:], in.TargetIp)
	err := rheincni.DeleteEndpoint(r.EndpointMap, key)
	if err != nil {
		return nil, fmt.Errorf("failed to delete a route: %w", err)
	}
	return &emptypb.Empty{}, nil
}

func (r *RouterService) AddVethPair(ctx context.Context, in *rsv1.AddVethPairReq) (*rsv1.AddVethPairResp, error) {
	var mp rheincni.MacPair // first is host mac, second is peer mac
	rheincni.GenerateRandomMacPair(&mp)
	peerIdx, err := rheincni.CreateVethPeer(in.HostEthName, in.PeerEthName, in.Netns, mp)
	if err != nil {
		return nil, fmt.Errorf("create veth peer veth: %w", err)
	}
	linkFd, err := loader.AttachProgramToInterface(r.ProgramFd, in.HostEthName, C.BPF_XDP)
	if err != nil {
		return nil, err
	}
	err = loader.PinObject(int(linkFd), BpfPinningBasePath+"/links/devices/"+in.HostEthName)
	if err != nil {
		return nil, err
	}
	hostVeth, err := net.InterfaceByName(in.HostEthName)
	if err != nil {
		return nil, err
	}
	return &rsv1.AddVethPairResp{
		HostMac: rheincni.MacToUint64(mp[0]),
		PeerMac: rheincni.MacToUint64(mp[1]),
		HostIdx: uint32(hostVeth.Index),
		PeerIdx: uint32(peerIdx),
	}, nil
}

func NewRouterService(routerProgramElf []byte, routerMapName string) (*RouterService, error) {
	if err := errors.Join(
		os.MkdirAll(BpfPinningBasePath+"/maps", 0700),
		os.MkdirAll(BpfPinningBasePath+"/programs", 0700),
		os.MkdirAll(BpfPinningBasePath+"/links/devices", 0700),
	); err != nil {
		return nil, fmt.Errorf("could not create needed bpffs directories: %w", err)
	}

	mapfd, err := loader.CreateAndPinOrGet(BpfPinningEndpointMap, func() (int, error) {
		fd, err := rheincni.CreateEndpointMap()
		if err != nil {
			return 0, err
		}
		return int(fd), nil
	})
	if err != nil {
		return nil, err
	}

	progfd, err := loader.CreateAndPinOrGet(BpfPinningRouterProgram, func() (int, error) {
		fd, err := loader.LoadProgram("GPL", routerProgramElf, C.BPF_PROG_TYPE_XDP,
			loader.OverrideMap(routerMapName, int(mapfd)))
		if err != nil {
			return 0, err
		}
		return int(fd), nil
	})
	if err != nil {
		return nil, err
	}

	return &RouterService{
		ProgramFd:   loader.ProgramFd(progfd),
		EndpointMap: rheincni.MapFd(mapfd),
	}, nil
}
