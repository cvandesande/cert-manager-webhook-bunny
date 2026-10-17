package bunnydns

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

const (
	testFqdn = "_acme-challenge.example.test."
	testKey  = "token-123"
)

// startFakeDNS starts an in-process UDP DNS server on 127.0.0.1 running h and
// returns its "host:port" address. The server is shut down at test cleanup.
func startFakeDNS(t *testing.T, h dns.HandlerFunc) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind UDP socket on 127.0.0.1: %v", err)
	}
	started := make(chan struct{})
	srv := &dns.Server{PacketConn: pc, Handler: h, NotifyStartedFunc: func() { close(started) }}
	go func() { _ = srv.ActivateAndServe() }()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

// txtReply makes a handler that always answers the request with a TXT record
// carrying value.
func txtReply(value string) dns.HandlerFunc {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		m.Authoritative = true
		m.Answer = []dns.RR{&dns.TXT{
			Hdr: dns.RR_Header{Name: req.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
			Txt: []string{value},
		}}
		_ = w.WriteMsg(m)
	}
}

// nxdomainReply makes a handler that always answers the request with NXDOMAIN.
func nxdomainReply() dns.HandlerFunc {
	return func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		m.Authoritative = true
		m.Rcode = dns.RcodeNameError
		_ = w.WriteMsg(m)
	}
}

func TestWaitForTXT_ImmediateSuccess(t *testing.T) {
	var queries atomic.Int32
	// Buffered so the handler records the first question without racing the
	// test goroutine that reads it back after WaitForTXT returns.
	type recorded struct {
		name  string
		qtype uint16
		rd    bool
	}
	captured := make(chan recorded, 1)

	addr := startFakeDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
		if queries.Add(1) == 1 {
			select {
			case captured <- recorded{req.Question[0].Name, req.Question[0].Qtype, req.RecursionDesired}:
			default:
			}
		}
		txtReply(testKey)(w, req)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// A huge interval proves WaitForTXT returns after the first round.
	if err := WaitForTXT(ctx, []string{addr}, testFqdn, testKey, time.Hour); err != nil {
		t.Fatalf("WaitForTXT returned %v, want nil", err)
	}
	if n := queries.Load(); n != 1 {
		t.Errorf("expected exactly 1 query, got %d", n)
	}
	var q recorded
	select {
	case q = <-captured:
	default:
		t.Fatal("handler did not record a query")
	}
	if q.name != testFqdn {
		t.Errorf("query name = %q, want %q", q.name, testFqdn)
	}
	if q.qtype != dns.TypeTXT {
		t.Errorf("query qtype = %d, want dns.TypeTXT (%d)", q.qtype, dns.TypeTXT)
	}
	if q.rd {
		t.Error("query RecursionDesired = true, want false")
	}
}

func TestWaitForTXT_AppearsAfterDelay(t *testing.T) {
	var queries atomic.Int32
	addr := startFakeDNS(t, func(w dns.ResponseWriter, req *dns.Msg) {
		if queries.Add(1) <= 3 {
			nxdomainReply()(w, req)
			return
		}
		txtReply(testKey)(w, req)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := WaitForTXT(ctx, []string{addr}, testFqdn, testKey, 5*time.Millisecond); err != nil {
		t.Fatalf("WaitForTXT returned %v, want nil", err)
	}
	if n := queries.Load(); n < 4 {
		t.Errorf("expected at least 4 queries, got %d", n)
	}
}

func TestWaitForTXT_AllServersRequired(t *testing.T) {
	addrA := startFakeDNS(t, txtReply(testKey))
	addrB := startFakeDNS(t, nxdomainReply())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := WaitForTXT(ctx, []string{addrA, addrB}, testFqdn, testKey, 10*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForTXT returned nil, want timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v does not wrap context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), addrB) {
		t.Errorf("error %q does not mention missing server %s", err, addrB)
	}
	if strings.Contains(err.Error(), addrA) {
		t.Errorf("error %q mentions server %s that has the record", err, addrA)
	}
}

func TestWaitForTXT_WrongValueIsNotAMatch(t *testing.T) {
	addr := startFakeDNS(t, txtReply("other-value"))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := WaitForTXT(ctx, []string{addr}, testFqdn, testKey, 10*time.Millisecond)
	if err == nil {
		t.Fatal("WaitForTXT returned nil, want timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v does not wrap context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "no matching TXT value") {
		t.Errorf("error %q does not mention %q", err, "no matching TXT value")
	}
}
