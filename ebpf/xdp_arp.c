#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/if_arp.h>
#include <bpf/bpf_helpers.h>

#include "../lib/types.h"

// To turn off just do not do anythiing with gateway_ip
const volatile ip  gateway_ip  = {0, 0, 0, 0};
const volatile mac gateway_mac = {0, 0, 0, 0, 0, 0};

SEC("xdp")
int xdp_arp_responder(struct xdp_md *ctx) {
    void *data     = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    if (eth->h_proto != __constant_htons(ETH_P_ARP))
        return XDP_PASS;

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

char _license[] SEC("license") = "GPL";
