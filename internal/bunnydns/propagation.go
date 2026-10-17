package bunnydns

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// queryTimeout bounds a single DNS query inside the propagation poller so one
// silent nameserver cannot stall a whole round.
const queryTimeout = 3 * time.Second

// AuthoritativeNameservers returns the authoritative nameserver addresses of
// zone as "host:port" strings (port 53), deduplicated and sorted ascending.
//
// zone may carry a trailing dot (e.g. "opendmz.com.").
func AuthoritativeNameservers(ctx context.Context, zone string) ([]string, error) {
	records, err := net.DefaultResolver.LookupNS(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to look up nameservers for zone %q: %w", zone, err)
	}
	seen := make(map[string]bool, len(records))
	var servers []string
	for _, ns := range records {
		// net.NS.Host may carry a trailing dot; JoinHostPort needs it gone.
		host := strings.TrimSuffix(ns.Host, ".")
		addr := net.JoinHostPort(host, "53")
		if !seen[addr] {
			seen[addr] = true
			servers = append(servers, addr)
		}
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("no nameservers found for zone %q", zone)
	}
	sort.Strings(servers)
	return servers, nil
}

// WaitForTXT polls the given servers (each a "host:port" address) until every
// one of them answers a TXT query for fqdn with a value exactly equal to key.
// It runs the first round immediately and waits interval between rounds; it
// returns ctx.Err() once ctx is done.
func WaitForTXT(ctx context.Context, servers []string, fqdn, key string, interval time.Duration) error {
	if len(servers) == 0 {
		return fmt.Errorf("no nameservers given to poll for TXT record %s", fqdn)
	}
	client := &dns.Client{Net: "udp", Timeout: queryTimeout}
	var lastMissing string
	for {
		missing, all := pollRound(ctx, client, servers, fqdn, key)
		if all {
			return nil
		}
		lastMissing = missing
		select {
		case <-ctx.Done():
			return fmt.Errorf("TXT record %s not visible on all nameservers (%s): %w", fqdn, lastMissing, ctx.Err())
		case <-time.After(interval):
		}
	}
}

// pollRound queries every server once and returns the "<server>: <reason>"
// entries for the servers that do not yet serve key, joined with "; ", plus
// whether all servers have the record.
func pollRound(ctx context.Context, client *dns.Client, servers []string, fqdn, key string) (missing string, all bool) {
	var parts []string
	for _, server := range servers {
		if reason, ok := hasTXT(ctx, client, server, fqdn, key); !ok {
			parts = append(parts, server+": "+reason)
		}
	}
	if len(parts) == 0 {
		return "", true
	}
	return strings.Join(parts, "; "), false
}

// hasTXT queries one server for the TXT record at fqdn and reports whether an
// answer matches key exactly. When it does not, the returned string explains
// why: "NXDOMAIN", "no matching TXT value", or the query error text.
func hasTXT(ctx context.Context, client *dns.Client, server, fqdn, key string) (reason string, ok bool) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(fqdn), dns.TypeTXT)
	m.RecursionDesired = false
	resp, _, err := client.ExchangeContext(ctx, m, server)
	if err != nil {
		return err.Error(), false
	}
	switch resp.Rcode {
	case dns.RcodeSuccess:
		// Matching answers are inspected below.
	case dns.RcodeNameError:
		return "NXDOMAIN", false
	default:
		return dns.RcodeToString[resp.Rcode], false
	}
	for _, rr := range resp.Answer {
		// A TXT record may be split into several strings; only the exact
		// concatenation counts as a match.
		if txt, isTXT := rr.(*dns.TXT); isTXT && strings.Join(txt.Txt, "") == key {
			return "", true
		}
	}
	return "no matching TXT value", false
}
