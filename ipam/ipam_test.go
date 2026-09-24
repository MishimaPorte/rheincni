package ipam

import (
	"context"
	"path/filepath"
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

	first := allocateForTest(t, service, 11)
	second := allocateForTest(t, service, 22)
	third := allocateForTest(t, service, 33)
	if first != 1 || second != 2 || third != 3 {
		t.Fatalf("allocated IPs = %d, %d, %d; want 1, 2, 3", first, second, third)
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

	if got := service.Subnet.List; got == nil || got.Start != 1 || got.End != 1 {
		t.Fatalf("first restored range = %#v; want [1,1]", got)
	}
	if got := service.Subnet.List.Next; got == nil || got.Start != 3 || got.End != 3 {
		t.Fatalf("second restored range = %#v; want [3,3]", got)
	}

	if reused := allocateForTest(t, service, 44); reused != second {
		t.Fatalf("reused IP = %d; want %d", reused, second)
	}
	if got := service.Subnet.List; got.Next != nil || got.Start != 1 || got.End != 3 {
		t.Fatalf("coalesced range = %#v; want [1,3]", got)
	}

	assertStoredAllocations(t, database, []uint32{1, 2, 3})
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
	if service.Subnet.List != nil {
		t.Fatalf("allocation was not rolled back: %#v", service.Subnet.List)
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

func assertStoredAllocations(t *testing.T, database *sqlite3.DB, want []uint32) {
	t.Helper()
	statement, err := database.Prepare("select ip from ip_allocation order by ip")
	if err != nil {
		t.Fatal(err)
	}
	defer statement.Close()

	var got []uint32
	for {
		hasRow, err := statement.Step()
		if err != nil {
			t.Fatal(err)
		}
		if !hasRow {
			break
		}
		got = append(got, uint32(statement.ColumnInt64(0)))
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
