package rheincni

import (
	"context"
	"fmt"
	"net"
	"rheincni/ipam/gen/ipamv1"
	"time"
)

// Command:1 ContainerId:5b8d8405c85f5365aad366c208a0ffe5740079459abcd4355b02d86d2cac2e25 Netns:/var/run/netns/cni-a0e79bb5-7455-63c7-c8eb-9e1f01b75ecb Ifname:eth0 Args:K8S_POD_NAMESPACE=kube-system;K8S_POD_NAME=coredns-559f6c778d-lw6z5;K8S_POD_INFRA_CONTAINER_ID=5b8d8405c85f5365aad366c208a0ffe5740079459abcd4355b02d86d2cac2e25;K8S_POD_UID=5ab7ad69-0092-4084-a33a-9bbec1e8406b;IgnoreUnknown=1 CniPath:/
func Add(e *EnvConfiguration, c *CniConfiguration, x ipamv1.IPAMServiceClient, out *Result) error {
	hostEthName := e.ContainerId[:16]
	var mp MacPair // first is host mac, second is peer mac
	GenerateRandomMacPair(&mp)
	err := CreateVethPeer(hostEthName, e.Ifname, e.Netns, mp)
	if err != nil {
		return fmt.Errorf("create veth peer veth: %w", err)
	}
	hostVeth, err := net.InterfaceByName(hostEthName)
	if err != nil {
		return fmt.Errorf("find host veth %s: %w", hostEthName, err)
	}

	ctx, cf := context.WithTimeout(context.Background(), time.Minute)
	defer cf()

	var ipamAlloc ipamv1.AllocateIPRequest
	ipamAlloc.HostEthIndex = int32(hostVeth.Index)
	allocation, err := x.AllocateIP(ctx, &ipamAlloc)

	if err != nil {
		panic("create veth peer veth: " + err.Error())
	}

	gw := IP(allocation.Gw)

	out.CniVersion = c.CniVersion
	out.Interfaces = append(out.Interfaces,
		Interface{
			Name: hostEthName,
			Mac:  mp[0],
		},
		Interface{
			Name:    e.Ifname,
			Mac:     mp[1],
			Sandbox: e.Netns,
		})
	out.Routes = append(out.Routes, Route{
		Dst: IPSubnet{IP: 0, Prefix: 0},
		Gw:  gw,
	})
	out.Ips = append(out.Ips, IPConfig{
		Address:   IP(allocation.Ip),
		Gateway:   gw,
		Interface: 1,
	})

	return nil
}
