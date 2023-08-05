package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/vpngen/realm-admin/internal/kdlib"
)

func main() {
	progName, err := os.Executable()
	if err != nil {
		log.Fatalf("Can't get executable name: %s\n", err)
	}

	token := os.Getenv("SUBDOMAPI_TOKEN")
	host := os.Getenv("SUBDOMAPI_HOST")

	if token == "" || host == "" {
		log.Fatalf("SUBDOMAPI_TOKEN or SUBDOMAPI_HOST is not set")
	}

	flag.Parse()

	if flag.NArg() < 1 {
		log.Fatalf("Usage: %s <pick|del> [subdomain]\n", progName)
	}

	switch flag.Arg(0) {
	case "pick":
		subdom, err := kdlib.SubdomainPick(host, token)
		if err != nil {
			log.Fatalf("Can't pick subdomain: %s\n", err)
		}

		fmt.Printf("%s\n", subdom)
	case "del":
		if flag.NArg() < 2 {
			log.Fatalf("Usage: %s del <subdomain>\n", progName)
		}

		if err := kdlib.SubdomainDelete(host, token, flag.Arg(1)); err != nil {
			log.Fatalf("Can't delete subdomain: %s\n", err)
		}
	default:
		log.Fatalf("Usage: %s <pick|del> [subdomain]\n", progName)
	}
}
