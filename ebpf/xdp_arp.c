#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/if_arp.h>
#include <bpf/bpf_helpers.h>

#include "../lib/types.h"

// To turn off just do not do anythiing with gateway_ip
const volatile ip  gateway_ip  = {0, 0, 0, 0};
const volatile mac gateway_mac = {0, 0, 0, 0, 0, 0};

typedef struct {
    u16 ar_hrd;  // format of hw address
    u16 ar_pro;  // format of proto address
    u8  ar_hln;  // length of hw address
    u8  ar_pln;  // length of proto address
    u16 ar_op;   // art opcode
    mac sender_mac;
    ip  sender_ip;
    mac target_mac;
    ip  target_ip;
} __attribute__((packed)) arp_hdr;

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

    if (__builtin_memcmp(&arp->target_ip[0], (void*)&gateway_ip[0], sizeof gateway_ip) != 0)
        return XDP_PASS;

    arp->ar_op = __constant_htons(ARPOP_REPLY);

    // target -> sender
    ip tmp_ip;
    __builtin_memcpy(&tmp_ip[0],         &arp->sender_ip[0], sizeof tmp_ip);
    __builtin_memcpy(&arp->sender_ip[0], &arp->target_ip[0], sizeof arp->sender_ip);
    __builtin_memcpy(&arp->target_ip[0], tmp_ip,             sizeof arp->target_ip);

    // target -> sender
    // sender -> gateway_mac
    __builtin_memcpy(&arp->target_mac[0], &arp->sender_mac[0], sizeof arp->target_mac);
    __builtin_memcpy(&arp->sender_mac[0], (void*)&gateway_mac[0],     sizeof arp->sender_mac);

    // eth dest -> sender
    // eth src  -> gateway
    __builtin_memcpy(eth->h_dest,   eth->h_source,   ETH_ALEN);
    __builtin_memcpy(eth->h_source, (void*)&gateway_mac[0], ETH_ALEN);

    return XDP_TX;
}

char _license[] SEC("license") = "GPL";
