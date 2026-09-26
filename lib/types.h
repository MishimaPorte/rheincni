#include <stdint.h>

typedef uint8_t  u8;
typedef uint16_t u16;
typedef uint32_t u32;
typedef uint64_t u64;

typedef int8_t  i8;
typedef int16_t i16;
typedef int32_t i32;
typedef int64_t i64;

typedef u8 ip[4];
typedef u8 mac[6];

typedef struct {
    i32 ifindex;
    mac mac;
} endpoint;

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

typedef struct {
#if   __BYTE_ORDER == __LITTLE_ENDIAN
    unsigned int ihl:4;
    unsigned int version:4;
#elif __BYTE_ORDER == __BIG_ENDIAN
    unsigned int version:4;
    unsigned int ihl:4;
#else
#  error "Please fix <bits/endian.h>"
#endif
    u8   tos;
    u16  tot_len;
    u16  id;
    u16  frag_off;
    u8   ttl;
    u8   protocol;
    u16  check;
    ip   saddr;
    ip   daddr;
} __attribute__((packed)) ip_hdr;
