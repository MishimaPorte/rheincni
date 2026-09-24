package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"rheincni"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var errPodCIDRPending = errors.New("node has no assigned PodCIDR")

type nodeGetter interface {
	Get(context.Context, string, metav1.GetOptions) (*corev1.Node, error)
}

func getNodePodCIDR(ctx context.Context, nodes nodeGetter, name string) (rheincni.IPSubnet, error) {
	if name == "" {
		return rheincni.IPSubnet{}, errors.New("NODE_NAME is empty")
	}
	node, err := nodes.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return rheincni.IPSubnet{}, fmt.Errorf("get Kubernetes node %q: %w", name, err)
	}

	cidrs := node.Spec.PodCIDRs
	if len(cidrs) == 0 && node.Spec.PodCIDR != "" {
		cidrs = []string{node.Spec.PodCIDR}
	}
	if len(cidrs) == 0 {
		return rheincni.IPSubnet{}, errPodCIDRPending
	}
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return rheincni.IPSubnet{}, fmt.Errorf("invalid PodCIDR %q on node %q: %w", cidr, name, err)
		}
		if prefix.Addr().Is4() {
			return rheincni.ParseIPSubnet(cidr)
		}
	}
	return rheincni.IPSubnet{}, fmt.Errorf("node %q has no IPv4 PodCIDR", name)
}

func waitForNodePodCIDR(ctx context.Context, nodes nodeGetter, name string) (rheincni.IPSubnet, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		subnet, err := getNodePodCIDR(ctx, nodes, name)
		if err == nil {
			return subnet, nil
		}
		if !errors.Is(err, errPodCIDRPending) {
			return rheincni.IPSubnet{}, err
		}
		select {
		case <-ctx.Done():
			return rheincni.IPSubnet{}, fmt.Errorf("wait for PodCIDR on node %q: %w", name, ctx.Err())
		case <-ticker.C:
		}
	}
}
