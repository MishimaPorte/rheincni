#include <linux/bpf.h>
#include <stddef.h>
#include <stdint.h>
#include <sys/syscall.h>
#include <unistd.h>

int bpf_create_map(int map_t,
                   size_t k_size,
                   size_t v_size,
                   size_t max_elems)
{
	union bpf_attr attr_create = {0};
	attr_create.map_type = map_t;
	attr_create.key_size = k_size;
	attr_create.value_size = v_size;
    if (max_elems)
        attr_create.max_entries = max_elems;
	int map_fd = syscall(__NR_bpf, BPF_MAP_CREATE, &attr_create, sizeof attr_create);
    if (map_fd > 0) return map_fd;
    return -1;
}

int bpf_update_map(int map_fd,
                   void *key,
                   void *value)
{
    union bpf_attr attr_update = {0};
    attr_update.map_fd = map_fd;
    attr_update.key = (uint64_t)key;
    attr_update.value = (uint64_t)value;
    attr_update.flags = BPF_ANY;
    return syscall(__NR_bpf, BPF_MAP_UPDATE_ELEM, &attr_update, sizeof attr_update);
}

int bpf_delete_map_element(int map_fd,
                           void *key)
{
    union bpf_attr attr_delete = {0};
    attr_delete.map_fd = map_fd;
    attr_delete.key = (uint64_t)key;
    attr_delete.flags = BPF_F_LOCK | BPF_ANY;
    return syscall(__NR_bpf, BPF_MAP_DELETE_ELEM, &attr_delete, sizeof attr_delete);
}
