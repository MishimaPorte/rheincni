#include <stdint.h>

#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <linux/if_ether.h>
#include <linux/if_arp.h>
#include <bpf/bpf_helpers.h>

#include "../lib/types.h"

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, ip);
    __type(value, endpoint);
} endpoints SEC(".maps");

const volatile mac this_mac = {0, 0, 0, 0, 0, 0};

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

    endpoint *value = bpf_map_lookup_elem(&endpoints, iph->daddr);
    if (!value)
        return XDP_PASS;

    // new hw_dest as needed
    __builtin_memcpy(eth->h_dest,    value->mac,      sizeof value->mac);
    __builtin_memcpy(eth->h_source,  (void*)this_mac, sizeof this_mac);
    return bpf_redirect(value->ifindex, 0);
}

char _license[] SEC("license") = "GPL";

