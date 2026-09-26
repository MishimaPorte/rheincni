package main

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type testNodeGetter struct {
	node *corev1.Node
	name string
}

func (g *testNodeGetter) Get(_ context.Context, name string, _ metav1.GetOptions) (*corev1.Node, error) {
	g.name = name
	return g.node, nil
}

func TestGetNodePodCIDRSelectsIPv4(t *testing.T) {
	getter := &testNodeGetter{node: &corev1.Node{Spec: corev1.NodeSpec{
		PodCIDRs: []string{"fd00::/64", "10.244.2.0/24"},
	}}}
	subnet, err := getNodePodCIDR(context.Background(), getter, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if getter.name != "worker-1" || subnet.String() != "10.244.2.0/24" {
		t.Fatalf("got node %q and subnet %s", getter.name, subnet)
	}
}

func TestGetNodePodCIDRFallsBackToSingularField(t *testing.T) {
	getter := &testNodeGetter{node: &corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "10.244.1.0/24"}}}
	subnet, err := getNodePodCIDR(context.Background(), getter, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if subnet.String() != "10.244.1.0/24" {
		t.Fatalf("got subnet %s", subnet)
	}
}

func TestGetNodePodCIDRWaitsForAssignment(t *testing.T) {
	getter := &testNodeGetter{node: &corev1.Node{}}
	_, err := getNodePodCIDR(context.Background(), getter, "worker-1")
	if !errors.Is(err, errPodCIDRPending) {
		t.Fatalf("got error %v; want pending PodCIDR", err)
	}
}

func TestGetNodePodCIDRRejectsInvalidRange(t *testing.T) {
	getter := &testNodeGetter{node: &corev1.Node{Spec: corev1.NodeSpec{PodCIDR: "not-a-cidr"}}}
	if _, err := getNodePodCIDR(context.Background(), getter, "worker-1"); err == nil {
		t.Fatal("accepted invalid PodCIDR")
	}
}
