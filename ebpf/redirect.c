#include <stdint.h>

#include <linux/bpf.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>

#include "../lib/types.h"

struct endpoint {
    u32 host_ifindex;
    mac pod_mac;
    mac host_mac;
};


struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, ip);
    __type(value, struct endpoint);
} host_endpoints SEC(".maps");


SEC("tc")
int route_packet(struct __sk_buff* skb) {
    print_thing(skb->len);
    aboba();

    int key = 0;
    int *value = bpf_map_lookup_elem(&icmpcnt, &key);
    bpf_printk("ok %d (pv: %d, ps: %s) (%d): %d\n", a, print_value, print_string, *value, bpf_get_current_cgroup_id());

    return TC_ACT_OK;
}

char _license[] SEC("license") = "GPL";

