package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"github.com/miekg/dns"
)

func main() {
	server := &dns.Server{Addr: "127.0.0.1:5353", Net: "udp"}
	dns.HandleFunc(".", handleDnsRequest)
	err := server.ListenAndServe()
	if err != nil {
		log.Fatalf("Failed to start server: %s\n", err.Error())
	}
}

func handleDnsRequest(w dns.ResponseWriter, r *dns.Msg) {
	buf, err := r.Pack()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't pack DNS message: %s\n", err)

		return
	}

	fi, err := os.Create("tmp_dns_question.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't create file: %s\n", err)

		return
	}

	defer fi.Close()

	fi.Write(buf)

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = false

	for _, q := range m.Question {
		sub, _, ok := strings.Cut(q.Name, ".")
		if !ok {
			continue
		}

		ip := strings.ReplaceAll(strings.Replace(sub, "w", "fd", 1), "s", ":")
		fmt.Fprintf(os.Stderr, "ip: %s\n", ip)

		switch q.Qtype {
		case dns.TypeAAAA:
			rr := new(dns.AAAA)
			rr.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 60}
			rr.AAAA = net.ParseIP(ip)
			m.Answer = append(m.Answer, rr)
		}
	}

	w.WriteMsg(m)

	buf, err = m.Pack()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't pack DNS message: %s\n", err)

		return
	}

	fo, err := os.Create("tmp_dns_answer.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't create file: %s\n", err)

		return
	}

	defer fo.Close()

	fo.Write(buf)
}
