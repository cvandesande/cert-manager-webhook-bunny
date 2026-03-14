// bunny-certbot-hook is a standalone certbot manual-hook binary that manages
// Bunny.net DNS TXT records for ACME DNS-01 challenges.
//
// Certbot invokes it twice per certificate request:
//
//	--manual-auth-hook    "bunny-certbot-hook present"
//	--manual-cleanup-hook "bunny-certbot-hook cleanup"
//
// API key — provide one of (checked in this order):
//
//	BUNNY_API_KEY       Bunny.net API access key (plain value)
//	BUNNY_API_KEY_FILE  Path to a file whose first line is the API key
//	/etc/bunny/api-key  Default key file location
//
// Environment variables set automatically by certbot:
//
//	CERTBOT_DOMAIN      Domain being validated (e.g. "example.com")
//	CERTBOT_VALIDATION  Value to place in the TXT record
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	bunny "github.com/simplesurance/bunny-go"

	"github.com/cvandesande/cert-manager-webhook-bunny/internal/bunnydns"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bunny-certbot-hook <present|cleanup>")
		os.Exit(1)
	}
	command := os.Args[1]

	apiKey, err := resolveAPIKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	domain, err := requireEnv("CERTBOT_DOMAIN")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	validation, err := requireEnv("CERTBOT_VALIDATION")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// The challenge record is always _acme-challenge.<domain>.
	fqdn := "_acme-challenge." + domain + "."

	client := bunny.NewClient(apiKey)
	ctx := context.Background()

	// Auto-discover which Bunny DNS zone covers this domain.
	// This handles subdomains: CERTBOT_DOMAIN=sub.example.com → zone "example.com."
	zone, _, err := bunnydns.FindZoneForDomain(ctx, client, domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bunny-certbot-hook: zone discovery failed: %v\n", err)
		os.Exit(1)
	}

	switch command {
	case "present":
		fmt.Printf("bunny-certbot-hook: setting TXT record %s (zone %s)\n", fqdn, zone)
		if err := bunnydns.PresentRecord(ctx, client, fqdn, zone, validation); err != nil {
			fmt.Fprintf(os.Stderr, "bunny-certbot-hook: present failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("bunny-certbot-hook: TXT record created successfully")

	case "cleanup":
		fmt.Printf("bunny-certbot-hook: removing TXT record %s (zone %s)\n", fqdn, zone)
		if err := bunnydns.CleanUpRecord(ctx, client, fqdn, zone, validation); err != nil {
			fmt.Fprintf(os.Stderr, "bunny-certbot-hook: cleanup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("bunny-certbot-hook: TXT record removed successfully")

	default:
		fmt.Fprintf(os.Stderr, "bunny-certbot-hook: unknown command %q (want present or cleanup)\n", command)
		os.Exit(1)
	}
}

const defaultKeyFile = "/etc/bunny/api-key"

// resolveAPIKey returns the Bunny.net API key using the following precedence:
//  1. BUNNY_API_KEY environment variable (plain value)
//  2. File named by BUNNY_API_KEY_FILE environment variable
//  3. Default key file at /etc/bunny/api-key
func resolveAPIKey() (string, error) {
	// 1. Plain env var
	if key := os.Getenv("BUNNY_API_KEY"); key != "" {
		return key, nil
	}
	// 2. Explicit file path
	if keyFile := os.Getenv("BUNNY_API_KEY_FILE"); keyFile != "" {
		return readKeyFile(keyFile)
	}
	// 3. Well-known default path
	if _, err := os.Stat(defaultKeyFile); err == nil {
		return readKeyFile(defaultKeyFile)
	}
	return "", fmt.Errorf(
		"Bunny.net API key not found — provide one of:\n" +
			"  BUNNY_API_KEY=<key>\n" +
			"  BUNNY_API_KEY_FILE=<path>\n" +
			"  or place the key in " + defaultKeyFile,
	)
}

// readKeyFile reads and returns the first non-empty line of the file at path.
func readKeyFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read key file %q: %w", path, err)
	}
	key := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if key == "" {
		return "", fmt.Errorf("key file %q is empty", path)
	}
	return key, nil
}

// requireEnv returns the value of the named environment variable, or an error
// if it is not set or empty.
func requireEnv(name string) (string, error) {
	val := os.Getenv(name)
	if val == "" {
		return "", fmt.Errorf("environment variable %s is required but not set", name)
	}
	return val, nil
}
