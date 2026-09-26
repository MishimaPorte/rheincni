package rheincni

// #include "map.h"
// #include "veth.h"
// #include "lib/types.h"
import "C"
import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

type MapFd int

func CreateEndpointMap() (MapFd, error) {
	fd := C.bpf_create_map(C.BPF_MAP_TYPE_HASH, C.sizeof_ip, C.sizeof_endpoint, 0)
	if fd > 0 {
		return MapFd(fd), nil
	}

	return 0, fmt.Errorf("map creation error: %w", syscall.Errno(C.get_errno()))
}

func UpdateEndpoint(mapFd MapFd, ip uint32, mac uint64, ifindex int) error {
	endpoint := C.endpoint{
		ifindex: C.int(ifindex),
	}
	binary.BigEndian.PutUint64(unsafe.Slice(
		(*byte)(unsafe.Pointer(&endpoint.mac[0])),
		C.sizeof_mac,
	), mac)
	err := C.bpf_update_map(C.int(mapFd), unsafe.Pointer(&ip), unsafe.Pointer(&endpoint))
	if err != 0 {
		return syscall.Errno(err)
	}
	return nil
}

func DeleteEndpoint(mapFd MapFd, ip uint32) error {
	err := C.bpf_delete_map_element(C.int(mapFd), unsafe.Pointer(&ip))
	if err != 0 {
		return syscall.Errno(err)
	}
	return nil
}
