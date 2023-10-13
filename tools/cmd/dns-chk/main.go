package main

import (
	"flag"
	"fmt"
	"log"
	"net/netip"
	"strings"

	dcmgmt "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"
)

func main() {
	domain := flag.String("d", "", "domain name")
	kd := flag.Bool("kd", false, "It is a keydesk address")
	ipstr := flag.String("ip", "", "IP address")
	srv := flag.String("ns", "", "nameservers, comma separated")

	flag.Parse()

	if (*domain == "" && !*kd) || (*domain != "" && *kd) || *ipstr == "" || *srv == "" {
		flag.Usage()

		log.Fatalln("Missing required arguments")
	}

	ip, err := netip.ParseAddr(*ipstr)
	if err != nil {
		log.Fatalf("Can't parse IP address: %s\n", err)
	}

	if *kd {
		if !ip.Is6() || !strings.HasPrefix(*ipstr, "fd") {
			log.Fatalf("Keydesk IP address must be IPv6 and start with fd\n")
		}

		*domain = strings.ReplaceAll(strings.Replace(*ipstr, (*ipstr)[:2], "w", 1), ":", "s") + ".vpn.works"
	}

	// servers := strings.Split(*srv, ",")

	fmt.Printf("domain: %s\n", *domain)
	fmt.Printf("ip: %s\n", *ipstr)
	fmt.Printf("servers: %s\n", *srv)

	ok, err := dcmgmt.CheckForPresence(*domain, ip, strings.Split(*srv, ",")...)
	if err != nil {
		log.Fatalf("Can't check for presence: %s\n", err)
	}

	switch ok {
	case true:
		fmt.Printf("Domain %s is present\n", *domain)
	case false:
		fmt.Printf("Domain %s is not present\n", *domain)
	}
}
