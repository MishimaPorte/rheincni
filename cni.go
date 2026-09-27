package rheincni

import (
	"context"
	"fmt"
	"rheincni/ipam/gen/ipamv1"
	rsv1 "rheincni/router_service/gen/routerservicev1"
	"time"
)

// Command:1 ContainerId:5b8d8405c85f5365aad366c208a0ffe5740079459abcd4355b02d86d2cac2e25 Netns:/var/run/netns/cni-a0e79bb5-7455-63c7-c8eb-9e1f01b75ecb Ifname:eth0 Args:K8S_POD_NAMESPACE=kube-system;K8S_POD_NAME=coredns-559f6c778d-lw6z5;K8S_POD_INFRA_CONTAINER_ID=5b8d8405c85f5365aad366c208a0ffe5740079459abcd4355b02d86d2cac2e25;K8S_POD_UID=5ab7ad69-0092-4084-a33a-9bbec1e8406b;IgnoreUnknown=1 CniPath:/
func Add(
	e *EnvConfiguration,
	c *CniConfiguration,
	ipam ipamv1.IPAMServiceClient,
	rs rsv1.RouterServiceClient,
	out *Result,
) error {
	hostEthName := e.ContainerId[:16]
	ctxMain, cfMain := context.WithCancel(context.Background())
	defer cfMain()

	ctx, cf := context.WithTimeout(ctxMain, time.Minute)
	ifInfo, err := rs.AddVethPair(ctx, &rsv1.AddVethPairReq{
		Netns:       e.Netns,
		PeerEthName: e.Ifname,
		HostEthName: hostEthName,
	})
	if err != nil {
		return fmt.Errorf("add veth pair error: %w", err)
	}
	cf()

	// Two subsequent operations can run in parallel probably
	// if we deal with ifindex dependency, but it is okay
	// for a prototype to run it sync.
	ctx, cf = context.WithTimeout(ctxMain, time.Minute)
	allocation, err := ipam.AllocateIP(ctx, &ipamv1.AllocateIPRequest{
		HostEthIndex: int32(ifInfo.HostIdx),
		PeerEthIndex: int32(ifInfo.PeerIdx),
		Netns:        e.Netns,
	})
	if err != nil {
		return fmt.Errorf("allocate ip error: %w", err)
	}
	cf()

	ctx, cf = context.WithTimeout(ctxMain, time.Minute)
	_, err = rs.AddRoute(ctx, &rsv1.AddRouteReq{
		TargetIp:      allocation.Ip,
		TargetIfindex: ifInfo.HostIdx,
		TargetMac:     ifInfo.PeerMac,
	})
	if err != nil {
		return fmt.Errorf("add route error: %w", err)
	}
	cf()

	gw := IP(allocation.Gw)

	out.CniVersion = c.CniVersion
	out.Interfaces = append(out.Interfaces,
		Interface{
			Name: hostEthName,
			Mac:  MacFromUint64(ifInfo.HostMac),
		},
		Interface{
			Name:    e.Ifname,
			Mac:     MacFromUint64(ifInfo.PeerMac),
			Sandbox: e.Netns,
		})
	out.Routes = append(out.Routes, Route{
		Dst: IPSubnet{IP: 0, Prefix: 0},
		Gw:  gw,
	})
	out.Ips = append(out.Ips, IPConfig{
		Address:   IPSubnet{IP: IP(allocation.Ip), Prefix: 32},
		Gateway:   gw,
		Interface: 1,
	})

	return nil
}
