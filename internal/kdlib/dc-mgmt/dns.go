package dcmgmt

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/miekg/dns"
)

const MAX_DNS_RETRIES = 3

var (
	ErrUnknownIP       = errors.New("unknown ip address")
	ErrNegativeRcode   = errors.New("negative rcode")
	ErrRetriesExceeded = errors.New("retries exceeded")
)

// Check for the presence of a record in the NS-servers
func CheckForPresence(fqdn string, ip netip.Addr, nameservers ...string) (bool, error) {
	// Make fqdn FQDN.
	if !strings.HasSuffix(fqdn, ".") {
		fqdn = fqdn + "."
	}

	// Create a new DNS message
	msg := &dns.Msg{}

	switch {
	case ip.Is4():
		msg.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)
	case ip.Is6():
		msg.SetQuestion(dns.Fqdn(fqdn), dns.TypeAAAA)
	default:
		return false, ErrUnknownIP
	}

	// Create a DNS exchange (client)
	client := &dns.Client{}

NS:
	for _, nameserver := range nameservers {
		if _, _, ok := strings.Cut(nameserver, ":"); !ok {
			nameserver = nameserver + ":53"
		}

		resp := &dns.Msg{}

		for attempt := 0; attempt < MAX_DNS_RETRIES; attempt++ {
			var err error

			// Perform the DNS exchange with the specific nameserver
			resp, _, err = client.Exchange(msg, nameserver)
			if err == nil && resp.Rcode != dns.RcodeRefused {
				break
			}

			if attempt == MAX_DNS_RETRIES-1 {
				return false, fmt.Errorf("exchange: %w", ErrRetriesExceeded)
			}
		}

		// Check the response status
		if resp.Rcode != dns.RcodeSuccess {
			return false, fmt.Errorf("rcode: %w: %s", ErrNegativeRcode, dns.RcodeToString[resp.Rcode])
		}

		// Parse the DNS answer
		for _, ans := range resp.Answer {
			switch {
			case ip.Is4():
				if rr, ok := ans.(*dns.A); ok {
					if rr.A.Equal(net.IP(ip.AsSlice())) {
						continue NS
					}
				}
			case ip.Is6():
				if rr, ok := ans.(*dns.AAAA); ok {
					if rr.AAAA.Equal(net.IP(ip.AsSlice())) {
						continue NS
					}
				}
			}
		}

		return false, nil
	}

	return true, nil
}
