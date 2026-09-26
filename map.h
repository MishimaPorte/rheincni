#if !defined(MAP_H)
#define MAP_H
#include <stddef.h>
#include <linux/bpf.h>

int bpf_create_map(int map_t,
                   size_t k_size,
                   size_t v_size,
                   size_t max_elems);
int bpf_update_map(int map_fd,
                   void *key,
                   void *value);
int bpf_delete_map_element(int map_fd,
                           void *key);
#endif
