package kdlib

import (
	"encoding/binary"
	"math/rand"
	"net/netip"
)

// kmaskCorrection for avoid negative value.
const maskCorrection = (^uint64(0)) >> 1

// IsZeroEnding - is last part zeroes.
func IsZeroEnding(ip netip.Addr) bool {
	if ip.Is4() && ip.As4()[3] == 0 {
		return true
	}

	if ip.Is6() {
		buf := ip.As16()
		return binary.BigEndian.Uint16(buf[14:]) == 0
	}

	return false
}

// RandomAddrIPv4 - random ipv4 addr inside prefix.
func RandomAddrIPv4(p netip.Prefix) netip.Addr {
	var buf [4]byte

	mp := p.Masked()
	m := (^uint32(0)) >> mp.Bits()
	a := binary.BigEndian.Uint32(mp.Addr().AsSlice())

	x := uint32(0)
	if m > 0 {
		x = uint32(rand.Int63n(int64(m)))
	}

	binary.BigEndian.PutUint32(buf[:], (x&m)|a)

	return netip.AddrFrom4(buf)
}

// RandomAddrIPv6 - random ipv6 addr inside prefix.
func RandomAddrIPv6(p netip.Prefix) netip.Addr {
	var buf [16]byte

	mp := p.Masked()
	s := mp.Addr().AsSlice()

	mLo := ^uint64(0)
	if mp.Bits() > 64 {
		mLo = (^uint64(0)) >> (mp.Bits() - 64)
	}

	randomFrom8(s[8:], buf[8:], mLo)

	if mp.Bits() < 64 {
		mHi := (^uint64(0)) >> mp.Bits()

		randomFrom8(s[:8], buf[:8], mHi)

		return netip.AddrFrom16(buf)
	}

	copy(buf[:8], s[:8])

	return netip.AddrFrom16(buf)
}

// src and dst MUST be len 8.
func randomFrom8(src, dst []byte, mask uint64) {
	addr := binary.BigEndian.Uint64(src[:8])

	x := uint64(0)
	if mask > 0 {
		x = uint64(rand.Int63n(int64(mask&maskCorrection)) * (-1 * (rand.Int63() % 2)))
	}

	binary.BigEndian.PutUint64(dst[:8], (x&mask)|addr)
}

// LastPrefixIPv4 - last ip addr in the prefix.
func LastPrefixIPv4(p netip.Prefix) netip.Addr {
	var buf [4]byte

	mp := p.Masked()
	addr0 := binary.BigEndian.Uint32(mp.Addr().AsSlice())
	addr1mask := (^uint32(0)) >> mp.Bits()

	binary.BigEndian.PutUint32(buf[:], addr0|addr1mask)

	return netip.AddrFrom4(buf)
}

// LastPrefixIPv6 - last ip addr in the prefix.
func LastPrefixIPv6(p netip.Prefix) netip.Addr {
	var buf [16]byte

	mp := p.Masked()
	s := mp.Addr().AsSlice()

	mLo := ^uint64(0)
	if mp.Bits() > 64 {
		mLo = (^uint64(0)) >> (mp.Bits() - 64)
	}

	binary.BigEndian.PutUint64(buf[8:], binary.BigEndian.Uint64(s[8:])|mLo)

	if mp.Bits() < 64 {
		mHi := (^uint64(0)) >> mp.Bits()

		binary.BigEndian.PutUint64(buf[:8], binary.BigEndian.Uint64(s[:8])|mHi)

		return netip.AddrFrom16(buf)
	}

	copy(buf[:8], s[:8])

	return netip.AddrFrom16(buf)
}
