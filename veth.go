package rheincni

import (
	"syscall"
	"unsafe"

	// #include "veth.h"
	// #include <net/if.h>
	// #include <errno.h>
	// #include <linux/rtnetlink.h>
	"C"
)
import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/netip"
	"runtime"

	"golang.org/x/sys/unix"
)

type Mac [6]byte

func (m Mac) String() string {
	return fmt.Sprintf(
		"%02x:%02x:%02x:%02x:%02x:%02x",
		m[0], m[1], m[2],
		m[3], m[4], m[5],
	)
}
func MacFromUint64(u uint64) Mac {
	var bts [8]byte
	binary.BigEndian.PutUint64(bts[:], u)
	return Mac(bts[:6])
}

func MacToUint64(m Mac) uint64 {
	var b [8]byte
	copy(b[:6], m[:])
	return binary.BigEndian.Uint64(b[:])
}

type MacPair [2]Mac

func (m MacPair) String() string {
	return fmt.Sprintf("Mac pair: [%s] and [%s]", m[0], m[1])
}

type IP uint32

func (i IP) String() string {
	return fmt.Sprintf(
		"%d.%d.%d.%d",
		byte(i>>24), byte(i>>16),
		byte(i>>8), byte(i),
	)
}

type IPSubnet struct {
	IP
	Prefix uint8
}

func ParseIPSubnet(cidr string) (IPSubnet, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return IPSubnet{}, fmt.Errorf("parse pod CIDR %q: %w", cidr, err)
	}
	if !prefix.Addr().Is4() {
		return IPSubnet{}, fmt.Errorf("pod CIDR %q is not IPv4", cidr)
	}
	if prefix.Bits() > 30 {
		return IPSubnet{}, fmt.Errorf("pod CIDR %q has no room for a gateway and a pod", cidr)
	}
	prefix = prefix.Masked()
	addr := prefix.Addr().As4()
	return IPSubnet{IP: IP(binary.BigEndian.Uint32(addr[:])), Prefix: uint8(prefix.Bits())}, nil
}

func (i IPSubnet) Top() IP {
	return i.Bottom() | IP(^(^uint32(0) << (32 - i.Prefix)))
}

func (i IPSubnet) Bottom() IP {
	return i.IP & IP(^uint32(0)<<(32-i.Prefix))
}

func (i IPSubnet) String() string {
	return fmt.Sprintf("%s/%d", i.IP, i.Prefix)
}

func GenerateRandomMacPair(out *MacPair) {
	// the crypto/rand.Read here is not needed,
	// but the deprecation warning is annoying
	rand.Read((*[unsafe.Sizeof(*out)]byte)(unsafe.Pointer(out))[:])

	// Set locally administered addresses bit and reset multicast bit
	out[0][0] = (out[0][0] | 0x02) & 0xfe
	out[1][0] = (out[1][0] | 0x02) & 0xfe

}

func CreateVethPeer(hostEth string, peerVeth string, netns string, mp MacPair) (peerIdx int, err error) {
	cstrHostVeth := C.CString(hostEth)
	cstrPeerVeth := C.CString(peerVeth)

	defer C.free(unsafe.Pointer(cstrHostVeth))
	defer C.free(unsafe.Pointer(cstrPeerVeth))

	netnsFd, err := syscall.Open(netns, syscall.O_RDONLY, 0)
	if err != nil && err.(syscall.Errno) != 0 {
		return 0, err
	}
	defer unix.Close(netnsFd)

	idx := C.create_veth_peer(cstrHostVeth, cstrPeerVeth, C.int(netnsFd), (*C.mac)(unsafe.Pointer(&mp[0])), (*C.mac)(unsafe.Pointer(&mp[1])))
	if idx == -1 {
		return 0, syscall.Errno(C.get_errno())
	}

	return int(idx), nil
}

func SetupPodIPRouting(ifindex int, podIfIndex int, podIp IP, gwIp IP, netns string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	sock, err := unix.Socket(
		unix.AF_NETLINK,
		unix.SOCK_RAW,
		unix.NETLINK_ROUTE,
	)
	if err != nil {
		return err
	}
	defer unix.Close(sock)

	isErr := C.veth_netlink_newroute(
		C.int(sock),
		C.int(ifindex),
		C.RT_SCOPE_LINK,
		C.RT_TABLE_MAIN,
		32,
		C.uint32_t(podIp),
		0)
	if isErr == -1 {
		return syscall.Errno(C.get_errno())
	}

	netnsFd, err := unix.Open(netns, unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(netnsFd)

	oldNs, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(oldNs)

	_, _, errno := syscall.Syscall(unix.SYS_SETNS, uintptr(netnsFd), 0, 0)
	if errno != 0 {
		return errno
	}
	defer func() {
		_, _, errno := syscall.Syscall(unix.SYS_SETNS, uintptr(oldNs), 0, 0)
		if errno != 0 {
			panic("could not restore netns: " + errno.Error())
		}
	}()

	podSock, err := unix.Socket(
		unix.AF_NETLINK,
		unix.SOCK_RAW,
		unix.NETLINK_ROUTE,
	)
	if err != nil {
		return err
	}
	defer unix.Close(podSock)

	isErr = C.veth_netlink_newaddr(
		C.int(podSock),
		C.int(podIfIndex),
		C.uint32_t(podIp))
	if isErr == -1 {
		return syscall.Errno(C.get_errno())
	}

	isErr = C.veth_netlink_newroute(
		C.int(podSock),
		C.int(podIfIndex),
		C.RT_SCOPE_LINK,
		C.RT_TABLE_MAIN,
		32,
		C.uint32_t(gwIp),
		0)
	if isErr == -1 {
		return syscall.Errno(C.get_errno())
	}
	isErr = C.veth_netlink_newroute(
		C.int(podSock),
		C.int(podIfIndex),
		C.RT_SCOPE_UNIVERSE,
		C.RT_TABLE_MAIN,
		0,
		0,
		C.uint32_t(gwIp))
	if isErr == -1 {
		return syscall.Errno(C.get_errno())
	}

	return nil
}
