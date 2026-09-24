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

const (
	ownerPod    = "pod"
	ownerRouter = "router"
)

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
	if err := migrateOwnerType(db); err != nil {
		return nil, err
	}

	service := &IPAMService{
		DB:     db,
		Subnet: SubnetAllocation{Subnet: subnet},
	}
	if err := service.loadAllocations(); err != nil {
		return nil, err
	}
	if service.GW == 0 {
		gw, ok := service.Subnet.Allocate()
		if !ok {
			return nil, fmt.Errorf("could not allocate an address for the gateway")
		}
		if gw != subnet.Bottom()+1 {
			service.Subnet.Deallocate(gw)
			return nil, fmt.Errorf("cannot reserve gateway address %s: it is already allocated to a pod", subnet.Bottom()+1)
		}
		if err := service.insertAllocation(gw, 0, ownerRouter); err != nil {
			service.Subnet.Deallocate(gw)
			return nil, fmt.Errorf("persist gateway reservation: %w", err)
		}
		service.GW = gw
	}
	return service, nil
}

// Existing databases predate owner_type. Their leases all belong to pods.
func migrateOwnerType(db *sqlite3.DB) error {
	statement, err := db.Prepare("pragma table_info(ip_allocation)")
	if err != nil {
		return fmt.Errorf("inspect IPAM schema: %w", err)
	}
	hasOwnerType := false
	for {
		hasRow, err := statement.Step()
		if err != nil {
			statement.Close()
			return fmt.Errorf("inspect IPAM schema: %w", err)
		}
		if !hasRow {
			break
		}
		if statement.ColumnTextView(1) == "owner_type" {
			hasOwnerType = true
		}
	}
	if err := statement.Close(); err != nil {
		return fmt.Errorf("close IPAM schema inspection: %w", err)
	}
	if !hasOwnerType {
		if err := db.Exec("alter table ip_allocation add column owner_type text not null default 'pod' check (owner_type in ('pod', 'router'))"); err != nil {
			return fmt.Errorf("migrate IPAM owner type: %w", err)
		}
	}
	if err := db.Exec("create unique index if not exists ip_allocation_one_router on ip_allocation(owner_type) where owner_type = 'router'"); err != nil {
		return fmt.Errorf("ensure unique router reservation: %w", err)
	}
	return nil
}

func (s *IPAMService) loadAllocations() error {
	statement, err := s.DB.Prepare("select ip, owner_type from ip_allocation order by ip")
	if err != nil {
		return fmt.Errorf("prepare allocation restore query: %w", err)
	}
	defer statement.Close()

	first := s.Subnet.Subnet.Bottom() + 1
	top := s.Subnet.Subnet.Top() - 1
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
		switch owner := statement.ColumnTextView(1); owner {
		case ownerRouter:
			if s.GW != 0 && s.GW != ip {
				return fmt.Errorf("restore stored allocation: multiple router reservations")
			}
			s.GW = ip
		case ownerPod:
		default:
			return fmt.Errorf("restore stored allocation: unknown owner type %q for IP %s", owner, ip)
		}
		if err := s.Subnet.AddAllocated(ip); err != nil {
			return fmt.Errorf("restore stored allocation: %w", err)
		}
	}
}

func (s *IPAMService) GetInfo(ctx context.Context, in *emptypb.Empty) (*ipamv1.IPAMInfo, error) {
	return &ipamv1.IPAMInfo{
		Subnet:       uint32(s.Subnet.Subnet.IP),
		SubnetPrefix: uint32(s.Subnet.Subnet.Prefix),
		Gw:           uint32(s.GW),
	}, nil
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
	if err := s.insertAllocation(newIP, request.GetHostEthIndex(), ownerPod); err != nil {
		if !s.Subnet.Deallocate(newIP) {
			panic("IPAM allocation rollback failed")
		}
		return nil, err
	}

	return &ipamv1.IP{Ip: uint32(newIP), Gw: uint32(s.GW)}, nil
}

func (s *IPAMService) insertAllocation(ip rheincni.IP, hostEthIndex int32, ownerType string) error {
	statement, err := s.DB.Prepare("insert into ip_allocation (ip, veth_index, owner_type) values (?, ?, ?)")
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
	if err := statement.BindText(3, ownerType); err != nil {
		return fmt.Errorf("bind allocation owner: %w", err)
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
	if ip == s.GW {
		return nil, fmt.Errorf("cannot deallocate router address %s", ip)
	}
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
	statement, err := s.DB.Prepare("delete from ip_allocation where ip = ? and owner_type = 'pod'")
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
