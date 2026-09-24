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
)

type Mac [6]byte

func (m Mac) String() string {
	return fmt.Sprintf(
		"%02x:%02x:%02x:%02x:%02x:%02x",
		m[0], m[1], m[2],
		m[3], m[4], m[5],
	)
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

func CreateVethPeer(hostEth string, peerVeth string, netns string, mp MacPair) (err error) {
	cstrHostVeth := C.CString(hostEth)
	cstrPeerVeth := C.CString(peerVeth)

	defer C.free(unsafe.Pointer(cstrHostVeth))
	defer C.free(unsafe.Pointer(cstrPeerVeth))

	netnsFd, err := syscall.Open(netns, syscall.O_RDONLY, 0)
	if err != nil && err.(syscall.Errno) != 0 {
		return err
	}

	if C.create_veth_peer(cstrHostVeth, cstrPeerVeth, C.int(netnsFd), (*C.mac)(unsafe.Pointer(&mp[0])), (*C.mac)(unsafe.Pointer(&mp[1]))) == -1 {
		return syscall.Errno(C.get_errno())
	}

	return nil
}
