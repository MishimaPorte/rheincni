#include <stdint.h>

#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>

#include "../lib/types.h"

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, ip);
    __type(value, endpoint);
} endpoints SEC(".maps");


SEC("xdp")
int route_packet(struct xdp_md *ctx) {

    void *data     = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    if (eth->h_proto != __constant_htons(ETH_P_IP))
        return XDP_PASS;

    ip_hdr *iph = (void*)(eth+1);

    endpoint *value = bpf_map_lookup_elem(&endpoints, &iph->daddr[0]);

    // // new ifindex as ne/*  */eded
    // ctx->egress_ifindex = endpoint->ifindex;
    // // new hw_dest as needed
    // __builtin_memcpy(eth->h_dest, &value->mac[0], sizeof value->mac);
    return bpf_redirect(ctx, value->ifindex);
}

char _license[] SEC("license") = "GPL";

