package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"

	bunny "github.com/simplesurance/bunny-go"
)

type bunnySolver struct {
	client *kubernetes.Clientset
}

type bunnyConfig struct {
	AccessKeySecretRef corev1.SecretKeySelector `json:"apiSecretRef"`
}

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}
	cmd.RunWebhookServer(GroupName,
		&bunnySolver{},
	)
}

func (c *bunnySolver) Name() string {
	return "bunny"
}

func (c *bunnySolver) Present(ch *v1alpha1.ChallengeRequest) error {
	log.Printf("Present: domain=%s fqdn=%s zone=%s", ch.DNSName, ch.ResolvedFQDN, ch.ResolvedZone)
	bunnyClient, err := c.newAPIClient(ch)
	if err != nil {
		return err
	}
	zoneID, err := c.resolveZoneId(bunnyClient, ch.ResolvedZone)
	if err != nil {
		return err
	}
	recordName := strings.TrimSuffix(strings.TrimSuffix(ch.ResolvedFQDN, ch.ResolvedZone), ".")
	val, err := c.hasTXTRecord(bunnyClient, recordName, ch.Key, zoneID)
	if err != nil {
		return err
	}
	if val != nil {
		log.Printf("Present: TXT record already exists for %s, skipping", ch.ResolvedFQDN)
		return nil
	}
	recordType := 3
	var ttl int32 = 120
	record := &bunny.AddOrUpdateDNSRecordOptions{
		Type:  &recordType,
		Value: &ch.Key,
		Name:  &recordName,
		TTL:   &ttl,
	}
	_, err = bunnyClient.DNSZone.AddDNSRecord(context.Background(), zoneID, record)
	if err != nil {
		return fmt.Errorf("failed to add TXT record: %s", err.Error())
	}
	log.Printf("Present: created TXT record %s in zone %s (zoneID=%d)", ch.ResolvedFQDN, ch.ResolvedZone, zoneID)
	return nil
}

func (c *bunnySolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	log.Printf("CleanUp: domain=%s fqdn=%s zone=%s", ch.DNSName, ch.ResolvedFQDN, ch.ResolvedZone)
	bunnyClient, err := c.newAPIClient(ch)
	if err != nil {
		return err
	}
	zoneID, err := c.resolveZoneId(bunnyClient, ch.ResolvedZone)
	if err != nil {
		return err
	}
	recordName := strings.TrimSuffix(strings.TrimSuffix(ch.ResolvedFQDN, ch.ResolvedZone), ".")
	record, err := c.hasTXTRecord(bunnyClient, recordName, ch.Key, zoneID)
	if err != nil {
		return fmt.Errorf("failed to get zone records: %v", err)
	}
	if record == nil {
		log.Printf("CleanUp: TXT record not found for %s, nothing to delete", ch.ResolvedFQDN)
		return nil
	}
	if err := bunnyClient.DNSZone.DeleteDNSRecord(context.Background(), zoneID, *record.ID); err != nil {
		return fmt.Errorf("failed to delete TXT record: %v", err)
	}
	log.Printf("CleanUp: deleted TXT record %s from zone %s (zoneID=%d)", ch.ResolvedFQDN, ch.ResolvedZone, zoneID)
	return nil
}

func (c *bunnySolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}
	c.client = cl
	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (bunnyConfig, error) {
	cfg := bunnyConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}
	return cfg, nil
}

func (c *bunnySolver) getAccessKeyFromSecret(ref corev1.SecretKeySelector, namespace string) (string, error) {
	if ref.Name == "" {
		return "", fmt.Errorf("undefined access key secret")
	}
	secret, err := c.client.CoreV1().Secrets(namespace).Get(context.TODO(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	accessKey, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key not found %q in secret '%s/%s'", ref.Key, namespace, ref.Name)
	}
	return string(accessKey), nil
}

func (c *bunnySolver) newAPIClient(ch *v1alpha1.ChallengeRequest) (*bunny.Client, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return nil, err
	}
	accessKey, err := c.getAccessKeyFromSecret(cfg.AccessKeySecretRef, ch.ResourceNamespace)
	if err != nil {
		return nil, err
	}
	return bunny.NewClient(accessKey), nil
}

func (c *bunnySolver) hasTXTRecord(client *bunny.Client, name, key string, zoneId int64) (*bunny.DNSRecord, error) {
	zone, err := client.DNSZone.Get(context.Background(), zoneId)
	if err != nil {
		return nil, fmt.Errorf("error getting zone records: %v", err)
	}
	for _, record := range zone.Records {
		if *record.Type == 3 && *record.Name == name && *record.Value == key {
			return &record, nil
		}
	}
	return nil, nil
}

func (c *bunnySolver) resolveZoneId(client *bunny.Client, zoneName string) (int64, error) {
	domain := strings.TrimSuffix(zoneName, ".")
	var i int32
	for i = 1; ; i++ {
		zones, err := client.DNSZone.List(context.Background(),
		    &bunny.PaginationOptions{
			Page: i,
			PerPage: 3,
		})
		if err != nil {
			return 0, err
		}
		for _, z := range zones.Items {
			if *z.Domain == domain {
				return *z.ID, nil
			}
		}
		if *zones.HasMoreItems == false {
			break
		}
	}
	return 0, fmt.Errorf("failed to get zone id from zone name: %s", zoneName)
}
