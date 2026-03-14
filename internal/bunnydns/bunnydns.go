// Package bunnydns provides helpers for managing DNS-01 challenge TXT records
// in Bunny.net DNS zones. It is shared between the cert-manager webhook server
// and the standalone certbot hook binary.
package bunnydns

import (
	"context"
	"fmt"
	"strings"

	bunny "github.com/simplesurance/bunny-go"
)

// PresentRecord creates a TXT record for an ACME DNS-01 challenge.
//
// fqdn is the fully-qualified record name (e.g. "_acme-challenge.example.com.").
// zone is the DNS zone name (e.g. "example.com.").
// key  is the ACME challenge value that must appear in the TXT record.
func PresentRecord(ctx context.Context, client *bunny.Client, fqdn, zone, key string) error {
	zoneID, err := ResolveZoneID(ctx, client, zone)
	if err != nil {
		return err
	}

	recordName := recordNameFromFQDN(fqdn, zone)

	existing, err := HasTXTRecord(ctx, client, recordName, key, zoneID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // already present, nothing to do
	}

	recordType := 3 // TXT
	var ttl int32 = 120
	record := &bunny.AddOrUpdateDNSRecordOptions{
		Type:  &recordType,
		Value: &key,
		Name:  &recordName,
		TTL:   &ttl,
	}
	if _, err = client.DNSZone.AddDNSRecord(ctx, zoneID, record); err != nil {
		return fmt.Errorf("failed to add TXT record: %w", err)
	}
	return nil
}

// CleanUpRecord removes the TXT record that was created for an ACME DNS-01 challenge.
//
// fqdn is the fully-qualified record name (e.g. "_acme-challenge.example.com.").
// zone is the DNS zone name (e.g. "example.com.").
// key  is the ACME challenge value that must appear in the TXT record.
func CleanUpRecord(ctx context.Context, client *bunny.Client, fqdn, zone, key string) error {
	zoneID, err := ResolveZoneID(ctx, client, zone)
	if err != nil {
		return err
	}

	recordName := recordNameFromFQDN(fqdn, zone)

	record, err := HasTXTRecord(ctx, client, recordName, key, zoneID)
	if err != nil {
		return fmt.Errorf("failed to look up TXT record: %w", err)
	}
	if record == nil {
		return nil // already gone, nothing to do
	}

	if err := client.DNSZone.DeleteDNSRecord(ctx, zoneID, *record.ID); err != nil {
		return fmt.Errorf("failed to delete TXT record: %w", err)
	}
	return nil
}

// ResolveZoneID returns the numeric Bunny.net zone ID for the given zone name.
// zoneName may include or omit a trailing dot.
func ResolveZoneID(ctx context.Context, client *bunny.Client, zoneName string) (int64, error) {
	domain := strings.TrimSuffix(zoneName, ".")
	var page int32
	for page = 1; ; page++ {
		zones, err := client.DNSZone.List(ctx, &bunny.PaginationOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return 0, fmt.Errorf("failed to list DNS zones: %w", err)
		}
		for _, z := range zones.Items {
			if *z.Domain == domain {
				return *z.ID, nil
			}
		}
		if !*zones.HasMoreItems {
			break
		}
	}
	return 0, fmt.Errorf("DNS zone %q not found in Bunny.net account", domain)
}

// HasTXTRecord checks whether a TXT record with the given name and value
// already exists in the zone. Returns the record if found, nil otherwise.
func HasTXTRecord(ctx context.Context, client *bunny.Client, name, key string, zoneID int64) (*bunny.DNSRecord, error) {
	zone, err := client.DNSZone.Get(ctx, zoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DNS zone records: %w", err)
	}
	for _, record := range zone.Records {
		if *record.Type == 3 && *record.Name == name && *record.Value == key {
			return &record, nil
		}
	}
	return nil, nil
}

// recordNameFromFQDN extracts the relative record name from a fully-qualified
// domain name by stripping the zone suffix and any trailing dot.
//
//	fqdn = "_acme-challenge.example.com."
//	zone = "example.com."
//	→    "_acme-challenge"
func recordNameFromFQDN(fqdn, zone string) string {
	return strings.TrimSuffix(strings.TrimSuffix(fqdn, zone), ".")
}
