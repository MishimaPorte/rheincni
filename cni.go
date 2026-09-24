package rheincni

import "fmt"

// Command:1 ContainerId:5b8d8405c85f5365aad366c208a0ffe5740079459abcd4355b02d86d2cac2e25 Netns:/var/run/netns/cni-a0e79bb5-7455-63c7-c8eb-9e1f01b75ecb Ifname:eth0 Args:K8S_POD_NAMESPACE=kube-system;K8S_POD_NAME=coredns-559f6c778d-lw6z5;K8S_POD_INFRA_CONTAINER_ID=5b8d8405c85f5365aad366c208a0ffe5740079459abcd4355b02d86d2cac2e25;K8S_POD_UID=5ab7ad69-0092-4084-a33a-9bbec1e8406b;IgnoreUnknown=1 CniPath:/
func Add(e *EnvConfiguration, c *CniConfiguration) error {
	hostEthName := e.ContainerId[:16]
	var mp MacPair // first is host mac, second is peer mac
	GenerateRandomMacPair(&mp)
	index, err := CreateVethPeer(hostEthName, e.Ifname, e.Netns, mp)
	if err != nil {
		return fmt.Errorf("create veth peer veth: %w", err.Error())
	}

	ip, err := AllocateIPFromLocalAgent("")
	if err != nil {
		panic("create veth peer veth: " + err.Error())
	}

	gw := ""

	var r Result
	r.CniVersion = c.CniVersion
	r.Interfaces = append(r.Interfaces,
		Interface{
			Name: hostEthName,
			Mac:  mp[0],
		},
		Interface{
			Name:    e.Ifname,
			Mac:     mp[1],
			Sandbox: e.Netns,
		})
	r.Routes = append(r.Routes, Route{
		Dst: "0.0.0.0/0",
		Gw:  gw,
	})
	r.Ips = append(r.Ips, Ip{
		Address:   ip,
		Gateway:   gw,
		Interface: index,
	})

	return nil
}

func AllocateIPFromLocalAgent(url string) (ip string, err error) {
	return "", nil
}
