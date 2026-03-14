package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"

	dns "github.com/cert-manager/cert-manager/test/acme"
)

var zone = os.Getenv("TEST_ZONE_NAME")

func TestRunsSuite(t *testing.T) {
	accessKey := os.Getenv("BUNNY_ACCESS_KEY")
	if accessKey == "" {
		t.Skip("BUNNY_ACCESS_KEY environment variable not set, skipping conformance tests")
	}
	if zone == "" {
		t.Skip("TEST_ZONE_NAME environment variable not set, skipping conformance tests")
	}

	// Write the credentials Secret to a temporary directory so that the
	// envtest cluster can apply it without requiring any checked-in credential files.
	tmpDir := t.TempDir()
	credYAML := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: bunny-credentials
type: Opaque
data:
  accessKey: %s
`, base64.StdEncoding.EncodeToString([]byte(accessKey)))

	if err := os.WriteFile(filepath.Join(tmpDir, "bunny-credentials.yaml"), []byte(credYAML), 0600); err != nil {
		t.Fatalf("failed to write credentials manifest: %v", err)
	}

	// Use authoritative mode by default: the test framework resolves the zone's
	// NS records and queries Bunny's own nameservers directly. This means:
	//   - No public resolver filtering / REFUSED errors
	//   - Records are visible immediately after creation (no propagation wait)
	// Override with TEST_USE_AUTHORITATIVE=false if needed.
	useAuthoritative := true
	if v := os.Getenv("TEST_USE_AUTHORITATIVE"); v == "false" {
		useAuthoritative = false
	}

	opts := []dns.Option{
		dns.SetResolvedZone(zone),
		dns.SetManifestPath(tmpDir),
		dns.SetUseAuthoritative(useAuthoritative),
		// Pass the solver config inline — no config.json file required.
		dns.SetConfig(bunnyConfig{
			AccessKeySecretRef: corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "bunny-credentials"},
				Key:                  "accessKey",
			},
		}),
	}

	// Allow an explicit DNS server override (only used when TEST_USE_AUTHORITATIVE=false).
	if dnsServer := os.Getenv("TEST_DNS_SERVER"); dnsServer != "" {
		opts = append(opts, dns.SetDNSServer(dnsServer))
	}

	fixture := dns.NewFixture(&bunnySolver{}, opts...)
	fixture.RunConformance(t)
}
