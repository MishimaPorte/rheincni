package rheincni

import "testing"

func TestParseIPSubnetBounds(t *testing.T) {
	subnet, err := ParseIPSubnet("10.244.1.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if subnet.String() != "10.244.1.0/24" || subnet.Bottom().String() != "10.244.1.0" || subnet.Top().String() != "10.244.1.255" {
		t.Fatalf("wrong subnet bounds: %s through %s for %s", subnet.Bottom(), subnet.Top(), subnet)
	}
}

func TestParseIPSubnetRejectsUnusableRanges(t *testing.T) {
	for _, cidr := range []string{"fd00::/64", "10.244.1.0/31", "invalid"} {
		if _, err := ParseIPSubnet(cidr); err == nil {
			t.Errorf("accepted %q", cidr)
		}
	}
}
