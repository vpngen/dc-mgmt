package kdlib

import (
	"net/netip"
	"testing"
)

type tLast struct {
	prefix, last string
}

var last4 = [...]tLast{
	{"0.0.0.0/0", "255.255.255.255"},
	{"192.168.1.0/24", "192.168.1.255"},
	{"255.255.255.255/32", "255.255.255.255"},
	{"0.0.0.0/16", "0.0.255.255"},
	{"127.0.0.1/32", "127.0.0.1"},
	{"192.168.4.0/23", "192.168.5.255"},
}

var last6 = [...]tLast{
	{"fd80::/64", "fd80::ffff:ffff:ffff:ffff"},
	{"fd80::/48", "fd80::ffff:ffff:ffff:ffff:ffff"},
	{"fd80::/80", "fd80::ffff:ffff:ffff"},
	{"::/0", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
	{"fd80::1/128", "fd80::1"},
	{"fd80::/65", "fd80::7fff:ffff:ffff:ffff"},
}

func TestLastPrefixIPv4(t *testing.T) {
	for _, v := range last4 {
		p := LastPrefixIPv4(netip.MustParsePrefix(v.prefix))
		if p.String() != v.last {
			t.Errorf("prefix: %s want last: %s got: %s", v.prefix, v.last, p.String())
		}
	}
}

func TestLastPrefixIPv6(t *testing.T) {
	for _, v := range last6 {
		p := LastPrefixIPv6(netip.MustParsePrefix(v.prefix))
		if p.String() != v.last {
			t.Errorf("prefix: %s want last: %s got: %s", v.prefix, v.last, p.String())
		}
	}
}

const attempts = 256

func TestRandomAddrIPv4(t *testing.T) {
	for _, v := range last4 {
		c := make(map[string]struct{})
		p := netip.MustParsePrefix(v.prefix)

		for i := 0; i < attempts; i++ {
			addr := RandomAddrIPv4(p)
			if !p.Contains(addr) {
				t.Errorf("prefix: %s got: %s", p.String(), addr.String())
			}

			c[addr.String()] = struct{}{}
		}

		if p.Bits() < 32 && len(c) < 2 {
			t.Errorf("prefix: %s have %d values: %v", p.String(), len(c), c)
		}

	}
}

func TestRandomAddrIPv6(t *testing.T) {
	for _, v := range last6 {
		c := make(map[string]struct{})
		p := netip.MustParsePrefix(v.prefix)

		for i := 0; i < attempts; i++ {
			addr := RandomAddrIPv6(p)
			if !p.Contains(addr) {
				t.Errorf("prefix: %s got: %s", p.String(), addr.String())
			}

			c[addr.String()] = struct{}{}
		}

		if p.Bits() < 128 && len(c) < 2 {
			t.Errorf("prefix: %s have %d values: %v", p.String(), len(c), c)
		}

	}
}

var zend = [...]struct {
	ip string
	ze bool
}{
	{"16.17.17.0", true},
	{"14.15.0.0", true},
	{"12.11.0.1", false},
	{"51.31.76.1", false},
	{"fd00::1", false},
	{"fd00:1::", true},
	{"fd00::ebab:1", false},
	{"fd00::ebab:0", true},
}

func TestIsZeroEnding(t *testing.T) {
	for _, v := range zend {
		if v.ze != IsZeroEnding(netip.MustParseAddr(v.ip)) {
			t.Errorf("%s is zero ending expected: %v", v.ip, v.ze)
		}
	}
}
