package ipam

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"rheincni"
	"rheincni/ipam/gen/ipamv1"
	sqlite3 "rheincni/thirdparty/sqlite/bindings"
)

func TestIPAMServicePersistsAndRestoresAllocations(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "ipam.db")
	database, err := sqlite3.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}

	subnet := rheincni.IPSubnet{IP: 0, Prefix: 24}
	service, err := NewIPAMService(database, subnet)
	if err != nil {
		t.Fatal(err)
	}
	if service.GW != 1 {
		t.Fatalf("gateway = %d; want 1", service.GW)
	}

	first := allocateForTest(t, service, 11)
	second := allocateForTest(t, service, 22)
	third := allocateForTest(t, service, 33)
	if first != 2 || second != 3 || third != 4 {
		t.Fatalf("allocated IPs = %d, %d, %d; want 2, 3, 4", first, second, third)
	}

	if _, err := service.DeallocateIP(context.Background(), &ipamv1.IP{Ip: second}); err != nil {
		t.Fatalf("deallocate second IP: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = sqlite3.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err = NewIPAMService(database, subnet)
	if err != nil {
		t.Fatal(err)
	}
	if service.GW != 1 {
		t.Fatalf("restored gateway = %d; want 1", service.GW)
	}

	if got := service.Subnet.List; got == nil || got.Start != 1 || got.End != 2 {
		t.Fatalf("first restored range = %#v; want [1,2]", got)
	}
	if got := service.Subnet.List.Next; got == nil || got.Start != 4 || got.End != 4 {
		t.Fatalf("second restored range = %#v; want [4,4]", got)
	}

	if reused := allocateForTest(t, service, 44); reused != second {
		t.Fatalf("reused IP = %d; want %d", reused, second)
	}
	if got := service.Subnet.List; got.Next != nil || got.Start != 1 || got.End != 4 {
		t.Fatalf("coalesced range = %#v; want [1,4]", got)
	}

	assertStoredAllocations(t, database, []storedAllocation{
		{1, ownerRouter}, {2, ownerPod}, {3, ownerPod}, {4, ownerPod},
	})
}

func TestAllocateRollsBackListWhenInsertFails(t *testing.T) {
	database, err := sqlite3.Open(filepath.Join(t.TempDir(), "ipam.db"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewIPAMService(database, rheincni.IPSubnet{IP: 0, Prefix: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := service.AllocateIP(context.Background(), &ipamv1.AllocateIPRequest{HostEthIndex: 1}); err == nil {
		t.Fatal("AllocateIP succeeded with a closed database")
	}
	if got := service.Subnet.List; got == nil || got.Start != service.GW || got.End != service.GW || got.Next != nil {
		t.Fatalf("pod allocation was not rolled back: %#v", got)
	}
}

func TestRouterReservationCannotBeDeallocated(t *testing.T) {
	database, err := sqlite3.Open(filepath.Join(t.TempDir(), "ipam.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err := NewIPAMService(database, rheincni.IPSubnet{IP: 0, Prefix: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeallocateIP(context.Background(), &ipamv1.IP{Ip: uint32(service.GW)}); err == nil {
		t.Fatal("deallocating the router address succeeded")
	}
	if pod := allocateForTest(t, service, 10); pod == uint32(service.GW) {
		t.Fatal("router address was allocated to a pod")
	}
	assertStoredAllocations(t, database, []storedAllocation{{1, ownerRouter}, {2, ownerPod}})
}

func TestLegacyAllocationMigration(t *testing.T) {
	database, err := sqlite3.Open(filepath.Join(t.TempDir(), "ipam.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Exec("create table ip_allocation (ip integer not null primary key, veth_index integer not null, container_id text, alloc_ts timestamp not null default current_timestamp)"); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("insert into ip_allocation (ip, veth_index) values (2, 11)"); err != nil {
		t.Fatal(err)
	}
	service, err := NewIPAMService(database, rheincni.IPSubnet{IP: 0, Prefix: 24})
	if err != nil {
		t.Fatal(err)
	}
	if service.GW != 1 {
		t.Fatalf("gateway = %d; want 1", service.GW)
	}
	assertStoredAllocations(t, database, []storedAllocation{{1, ownerRouter}, {2, ownerPod}})
}

func TestLegacyPodAtGatewayIsRejected(t *testing.T) {
	database, err := sqlite3.Open(filepath.Join(t.TempDir(), "ipam.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Exec("create table ip_allocation (ip integer not null primary key, veth_index integer not null)"); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("insert into ip_allocation (ip, veth_index) values (1, 11)"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewIPAMService(database, rheincni.IPSubnet{IP: 0, Prefix: 24}); err == nil {
		t.Fatal("startup succeeded despite a pod lease at the gateway address")
	}
	assertStoredAllocations(t, database, []storedAllocation{{1, ownerPod}})
}

func TestLoadAllocationsPreservesExistingRanges(t *testing.T) {
	database, err := sqlite3.Open(filepath.Join(t.TempDir(), "ipam.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err := NewIPAMService(database, rheincni.IPSubnet{IP: 0, Prefix: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Subnet.AddAllocated(5); err != nil {
		t.Fatal(err)
	}
	allocateForTest(t, service, 11)
	if err := service.loadAllocations(); err != nil {
		t.Fatal(err)
	}
	if got := service.Subnet.List; got == nil || got.Start != 1 || got.End != 2 || got.Next == nil || got.Next.Start != 5 {
		t.Fatalf("restoration replaced an existing range: %#v", got)
	}
}

func allocateForTest(t *testing.T, service *IPAMService, hostEthIndex int32) uint32 {
	t.Helper()
	result, err := service.AllocateIP(context.Background(), &ipamv1.AllocateIPRequest{HostEthIndex: hostEthIndex})
	if err != nil {
		t.Fatal(err)
	}
	return result.GetIp()
}

type storedAllocation struct {
	ip    uint32
	owner string
}

func assertStoredAllocations(t *testing.T, database *sqlite3.DB, want []storedAllocation) {
	t.Helper()
	statement, err := database.Prepare("select ip, owner_type from ip_allocation order by ip")
	if err != nil {
		t.Fatal(err)
	}
	defer statement.Close()

	var got []storedAllocation
	for {
		hasRow, err := statement.Step()
		if err != nil {
			t.Fatal(err)
		}
		if !hasRow {
			break
		}
		got = append(got, storedAllocation{uint32(statement.ColumnInt64(0)), strings.Clone(statement.ColumnTextView(1))})
	}
	if len(got) != len(want) {
		t.Fatalf("stored IPs = %v; want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("stored IPs = %v; want %v", got, want)
		}
	}
}
