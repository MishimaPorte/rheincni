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

const volatile ip  gateway_ip  = {0, 0, 0, 0};
const volatile mac gateway_mac = {0, 0, 0, 0, 0, 0};

const volatile mac this_mac = {0, 0, 0, 0, 0, 0};

int __route_ip(struct xdp_md *ctx, struct ethhdr *eth)
{
    ip_hdr *iph = (void*)(eth+1);
    endpoint *value = bpf_map_lookup_elem(&endpoints, iph->daddr);
    if (!value)
        return XDP_PASS;

    // new hw_dest as needed
    __builtin_memcpy(eth->h_dest,    value->mac,      sizeof value->mac);
    __builtin_memcpy(eth->h_source,  (void*)this_mac, sizeof this_mac);
    return bpf_redirect(value->ifindex, 0);
}

int __answer_arp(struct xdp_md *ctx, struct ethhdr *eth)
{
    void *data_end = (void *)(long)ctx->data_end;

    arp_hdr *arp = (void *)(eth + 1);
    if ((void *)(arp + 1) > data_end)
        return XDP_PASS;

    if (arp->ar_op != __constant_htons(ARPOP_REQUEST))
        return XDP_PASS;

    if (__builtin_memcmp(arp->target_ip, (void*)gateway_ip, sizeof gateway_ip) != 0)
        return XDP_PASS;

    arp->ar_op = __constant_htons(ARPOP_REPLY);

    // target -> sender
    ip tmp_ip;
    __builtin_memcpy(tmp_ip,         arp->sender_ip, sizeof tmp_ip);
    __builtin_memcpy(arp->sender_ip, arp->target_ip, sizeof arp->sender_ip);
    __builtin_memcpy(arp->target_ip, tmp_ip,         sizeof arp->target_ip);

    // target -> sender
    // sender -> gateway_mac
    __builtin_memcpy(arp->target_mac, arp->sender_mac,    sizeof arp->target_mac);
    __builtin_memcpy(arp->sender_mac, (void*)gateway_mac, sizeof arp->sender_mac);

    // eth dest -> sender
    // eth src  -> gateway
    __builtin_memcpy(eth->h_dest,   eth->h_source,      sizeof eth->h_dest);
    __builtin_memcpy(eth->h_source, (void*)gateway_mac, sizeof eth->h_source);

    return XDP_TX;
}

SEC("xdp")
int route_packet(struct xdp_md *ctx) {

    void *data     = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    switch (eth->h_proto) {
        case __constant_htons(ETH_P_IP):
            return __route_ip(ctx, eth);
        case __constant_htons(ETH_P_ARP):
            return __answer_arp(ctx, eth);
        default:
            return XDP_PASS;
    }
}

char _license[] SEC("license") = "GPL";

