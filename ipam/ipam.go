package ipam

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"rheincni"
	"rheincni/ipam/gen/ipamv1"
	sqlite3 "rheincni/thirdparty/sqlite/bindings"

	"google.golang.org/protobuf/types/known/emptypb"
)

//go:embed db.sql
var schemaSQL string

type IPRangeNode struct {
	Next  *IPRangeNode
	Prev  *IPRangeNode
	Start rheincni.IP
	End   rheincni.IP
}

type SubnetAllocation struct {
	Subnet rheincni.IPSubnet
	List   *IPRangeNode
}

func (s *SubnetAllocation) Allocate() (rheincni.IP, bool) {
	first := s.Subnet.Bottom() + 1
	top := s.Subnet.Top() - 1 // account for the broadcast address
	if first > top {
		return 0, false
	}

	if s.List == nil {
		s.List = &IPRangeNode{Start: first, End: first}
		return first, true
	}

	if first < s.List.Start {
		if first+1 == s.List.Start {
			s.List.Start = first
		} else {
			s.List = &IPRangeNode{Next: s.List, Start: first, End: first}
			s.List.Next.Prev = s.List
		}
		return first, true
	}

	for node := s.List; node != nil; node = node.Next {
		if node.End >= top {
			return 0, false
		}

		ip := node.End + 1
		if node.Next != nil && ip >= node.Next.Start {
			continue
		}

		node.End = ip
		if node.Next != nil && ip+1 == node.Next.Start {
			next := node.Next
			node.End = next.End
			node.Next = next.Next
			if node.Next != nil {
				node.Next.Prev = node
			}
		}
		return ip, true
	}

	return 0, false
}

// Deallocate releases ip from the allocated ranges.
func (s *SubnetAllocation) Deallocate(ip rheincni.IP) bool {
	for node := s.List; node != nil; node = node.Next {
		if ip < node.Start {
			return false
		}
		if ip > node.End {
			continue
		}

		switch {
		case node.Start == node.End:
			if node.Prev == nil {
				s.List = node.Next
			} else {
				node.Prev.Next = node.Next
			}
			if node.Next != nil {
				node.Next.Prev = node.Prev
			}
		case ip == node.Start:
			node.Start++
		case ip == node.End:
			node.End--
		default:
			right := &IPRangeNode{
				Prev:  node,
				Next:  node.Next,
				Start: ip + 1,
				End:   node.End,
			}
			if right.Next != nil {
				right.Next.Prev = right
			}
			node.End = ip - 1
			node.Next = right
		}
		return true
	}

	return false
}

// AddAllocated adds an existing allocation while preserving sorted,
// coalesced ranges. It is used when rebuilding state from SQLite.
func (s *SubnetAllocation) AddAllocated(ip rheincni.IP) error {
	first := s.Subnet.Bottom() + 1
	if ip < first || ip > s.Subnet.Top() {
		return fmt.Errorf("allocated IP %s is outside subnet %s", ip, s.Subnet)
	}

	var previous *IPRangeNode
	for node := s.List; node != nil; node = node.Next {
		if ip >= node.Start && ip <= node.End {
			return nil
		}
		if ip < node.Start {
			inserted := &IPRangeNode{Prev: previous, Next: node, Start: ip, End: ip}
			node.Prev = inserted
			if previous == nil {
				s.List = inserted
			} else {
				previous.Next = inserted
			}
			s.coalesce(inserted)
			return nil
		}
		previous = node
	}

	inserted := &IPRangeNode{Prev: previous, Start: ip, End: ip}
	if previous == nil {
		s.List = inserted
	} else {
		previous.Next = inserted
	}
	s.coalesce(inserted)
	return nil
}

func (s *SubnetAllocation) coalesce(node *IPRangeNode) {
	if node.Prev != nil && node.Prev.End+1 == node.Start {
		previous := node.Prev
		previous.End = node.End
		previous.Next = node.Next
		if node.Next != nil {
			node.Next.Prev = previous
		}
		node = previous
	}
	if node.Next != nil && node.End+1 == node.Next.Start {
		next := node.Next
		node.End = next.End
		node.Next = next.Next
		if node.Next != nil {
			node.Next.Prev = node
		}
	}
}

type IPAMService struct {
	DB     *sqlite3.DB
	Subnet SubnetAllocation
	GW     rheincni.IP

	mu sync.Mutex
	ipamv1.UnimplementedIPAMServiceServer
}

// NewIPAMService initializes the database schema and reconstructs the
// allocated-range list from leases left by previous process instances.
func NewIPAMService(db *sqlite3.DB, subnet rheincni.IPSubnet) (*IPAMService, error) {
	if db == nil {
		return nil, fmt.Errorf("IPAM database must not be nil")
	}
	if err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("initialize IPAM database: %w", err)
	}

	service := &IPAMService{
		DB:     db,
		Subnet: SubnetAllocation{Subnet: subnet},
	}
	gw, ok := service.Subnet.Allocate()
	if !ok {
		return nil, fmt.Errorf("could not allocate an address for the gateway")
	}
	if gw != service.Subnet.Subnet.Bottom()+1 {
		return nil, fmt.Errorf("could not allocate a bottom address for the gateway")
	}
	service.GW = gw
	if err := service.loadAllocations(); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *IPAMService) loadAllocations() error {
	statement, err := s.DB.Prepare("select ip from ip_allocation order by ip")
	if err != nil {
		return fmt.Errorf("prepare allocation restore query: %w", err)
	}
	defer statement.Close()

	first := s.Subnet.Subnet.Bottom() + 1
	top := s.Subnet.Subnet.Top()
	var tail *IPRangeNode
	for {
		hasRow, err := statement.Step()
		if err != nil {
			return fmt.Errorf("read stored allocations: %w", err)
		}
		if !hasRow {
			return nil
		}
		storedIP := statement.ColumnInt64(0)
		if storedIP < 0 || storedIP > int64(^uint32(0)) {
			return fmt.Errorf("restore stored allocation: IP integer %d is outside the IPv4 range", storedIP)
		}
		ip := rheincni.IP(storedIP)
		if ip < first || ip > top {
			return fmt.Errorf("restore stored allocation: allocated IP %s is outside subnet %s", ip, s.Subnet.Subnet)
		}
		if tail == nil {
			tail = &IPRangeNode{Start: ip, End: ip}
			s.Subnet.List = tail
			continue
		}
		if ip == tail.End+1 {
			tail.End = ip
			continue
		}
		next := &IPRangeNode{Prev: tail, Start: ip, End: ip}
		tail.Next = next
		tail = next
	}
}

func (s *IPAMService) AllocateIP(ctx context.Context, request *ipamv1.AllocateIPRequest) (*ipamv1.IP, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("allocate request must not be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	newIP, ok := s.Subnet.Allocate()
	if !ok {
		return nil, fmt.Errorf("out of IPv4 addresses")
	}
	if err := s.insertAllocation(newIP, request.GetHostEthIndex()); err != nil {
		if !s.Subnet.Deallocate(newIP) {
			panic("IPAM allocation rollback failed")
		}
		return nil, err
	}

	return &ipamv1.IP{Ip: uint32(newIP), Gw: uint32(s.GW)}, nil
}

func (s *IPAMService) insertAllocation(ip rheincni.IP, hostEthIndex int32) error {
	statement, err := s.DB.Prepare("insert into ip_allocation (ip, veth_index) values (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare allocation insert: %w", err)
	}
	defer statement.Close()

	if err := statement.BindInt64(1, int64(ip)); err != nil {
		return fmt.Errorf("bind allocated IP: %w", err)
	}
	if err := statement.BindInt64(2, int64(hostEthIndex)); err != nil {
		return fmt.Errorf("bind host interface index: %w", err)
	}
	if _, err := statement.Step(); err != nil {
		return fmt.Errorf("insert allocation for IP %s: %w", ip, err)
	}
	return nil
}

func (s *IPAMService) DeallocateIP(ctx context.Context, request *ipamv1.IP) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, fmt.Errorf("deallocate request must not be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ip := rheincni.IP(request.Ip)
	if !s.Subnet.Deallocate(ip) {
		return &emptypb.Empty{}, nil
	}
	if err := s.deleteAllocation(ip); err != nil {
		if rollbackErr := s.Subnet.AddAllocated(ip); rollbackErr != nil {
			panic(fmt.Sprintf("IPAM deallocation rollback failed: %v", rollbackErr))
		}
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *IPAMService) deleteAllocation(ip rheincni.IP) error {
	statement, err := s.DB.Prepare("delete from ip_allocation where ip = ?")
	if err != nil {
		return fmt.Errorf("prepare allocation delete: %w", err)
	}
	defer statement.Close()

	if err := statement.BindInt64(1, int64(ip)); err != nil {
		return fmt.Errorf("bind deallocated IP: %w", err)
	}
	if _, err := statement.Step(); err != nil {
		return fmt.Errorf("delete allocation for IP %s: %w", ip, err)
	}
	if s.DB.Changes() != 1 {
		return fmt.Errorf("allocation for IP %s was not present in the database", ip)
	}
	return nil
}
