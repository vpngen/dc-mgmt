package dcmgmt

import (
	"errors"
	"fmt"
	"net/netip"

	"github.com/vpngen/dc-mgmt/internal/kdlib"
)

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
)

const DefaultRandomAttemts = 10000

var ErrRandomAttemptsExceeded = errors.New("random attempts exceeded")

var (
	cgnatNetWindow netip.Prefix = netip.MustParsePrefix("100.64.0.0/10")
	reservedCGNATs              = []netip.Prefix{
		netip.MustParsePrefix("100.125.0.0/16"),
		netip.MustParsePrefix("100.126.0.0/16"),
		netip.MustParsePrefix("100.127.0.0/16"),
	}

	ulaNetWindow netip.Prefix = netip.MustParsePrefix("fd40::/10")
	reservedULAs              = []netip.Prefix{}

	keydeskNetWindow netip.Prefix = netip.MustParsePrefix("fdc0::/10")
	reservedKeydesks              = []netip.Prefix{}
)

func RandomCGNAT24Net() (netip.Prefix, error) {
	var cgnatNet netip.Prefix
CGNATLOOP:
	for attempts := 0; ; attempts++ {
		if attempts > DefaultRandomAttemts {
			return netip.Prefix{}, ErrRandomAttemptsExceeded
		}

		addr := kdlib.RandomAddrIPv4(cgnatNetWindow)
		if kdlib.IsZeroEnding(addr) {
			continue
		}

		cgnatNet = netip.PrefixFrom(addr, BrigadeCgnatPrefix)

		masked := cgnatNet.Masked()
		if masked.Addr() == addr || kdlib.LastPrefixIPv4(masked) == addr {
			continue
		}

		for _, reserved := range reservedCGNATs {
			if masked.Overlaps(reserved) {
				continue CGNATLOOP
			}
		}

		break
	}

	return cgnatNet, nil
}

func RandomULA64Net() (netip.Prefix, error) {
	var ulaNet netip.Prefix

ULALOOP:
	for attempts := 0; ; attempts++ {
		if attempts > DefaultRandomAttemts {
			return netip.Prefix{}, ErrRandomAttemptsExceeded
		}

		addr := kdlib.RandomAddrIPv6(ulaNetWindow)
		if kdlib.IsZeroEnding(addr) {
			continue
		}

		ulaNet = netip.PrefixFrom(addr, BrigadeUlaPrefix)

		masked := ulaNet.Masked()
		if masked.Addr() == addr || kdlib.LastPrefixIPv6(masked) == addr {
			continue
		}

		for _, reserved := range reservedULAs {
			if masked.Overlaps(reserved) {
				continue ULALOOP
			}
		}

		break
	}

	return ulaNet, nil
}

func RandomKeydesk() (netip.Addr, error) {
	var keydesk netip.Addr
KDLOOP:
	for attempts := 0; ; attempts++ {
		if attempts > DefaultRandomAttemts {
			return netip.Addr{}, fmt.Errorf("keydesk: %w", ErrRandomAttemptsExceeded)
		}

		keydesk = kdlib.RandomAddrIPv6(keydeskNetWindow)
		if kdlib.IsZeroEnding(keydesk) {
			continue
		}

		for _, reserved := range reservedKeydesks {
			if reserved.Contains(keydesk) {
				continue KDLOOP
			}
		}

		break
	}

	return keydesk, nil
}
