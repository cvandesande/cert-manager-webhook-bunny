// bunny-certbot-hook is a standalone certbot manual-hook binary that manages
// Bunny.net DNS TXT records for ACME DNS-01 challenges.
//
// Certbot invokes it twice per certificate request:
//
//	--manual-auth-hook    "bunny-certbot-hook present"
//	--manual-cleanup-hook "bunny-certbot-hook cleanup"
//
// API key — provide one of:
//
//	BUNNY_API_KEY       Bunny.net API access key (plain value)
//	BUNNY_API_KEY_FILE  Path to a file whose first line is the API key
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

	// Derive the FQDN and zone in the dotted form that bunnydns expects.
	// Certbot gives us "example.com"; we need "_acme-challenge.example.com." and "example.com."
	fqdn := "_acme-challenge." + domain + "."
	zone := domain + "."

	client := bunny.NewClient(apiKey)
	ctx := context.Background()

	switch command {
	case "present":
		fmt.Printf("bunny-certbot-hook: setting TXT record %s\n", fqdn)
		if err := bunnydns.PresentRecord(ctx, client, fqdn, zone, validation); err != nil {
			fmt.Fprintf(os.Stderr, "bunny-certbot-hook: present failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("bunny-certbot-hook: TXT record created successfully")

	case "cleanup":
		fmt.Printf("bunny-certbot-hook: removing TXT record %s\n", fqdn)
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

// resolveAPIKey returns the Bunny.net API key from the environment.
// It checks BUNNY_API_KEY first; if that is unset it reads the first line of
// the file named by BUNNY_API_KEY_FILE.
func resolveAPIKey() (string, error) {
	if key := os.Getenv("BUNNY_API_KEY"); key != "" {
		return key, nil
	}
	keyFile := os.Getenv("BUNNY_API_KEY_FILE")
	if keyFile == "" {
		return "", fmt.Errorf("environment variable BUNNY_API_KEY is required but not set\n" +
			"  (alternatively set BUNNY_API_KEY_FILE to a file containing the key)")
	}
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return "", fmt.Errorf("failed to read BUNNY_API_KEY_FILE %q: %w", keyFile, err)
	}
	key := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	if key == "" {
		return "", fmt.Errorf("BUNNY_API_KEY_FILE %q is empty", keyFile)
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
