package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"

	bunny "github.com/simplesurance/bunny-go"

	"github.com/cvandesande/cert-manager-webhook-bunny/internal/bunnydns"
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
	if err := bunnydns.PresentRecord(context.Background(), bunnyClient, ch.ResolvedFQDN, ch.ResolvedZone, ch.Key); err != nil {
		return err
	}
	log.Printf("Present: TXT record created/verified for %s in zone %s", ch.ResolvedFQDN, ch.ResolvedZone)
	return nil
}

func (c *bunnySolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	log.Printf("CleanUp: domain=%s fqdn=%s zone=%s", ch.DNSName, ch.ResolvedFQDN, ch.ResolvedZone)
	bunnyClient, err := c.newAPIClient(ch)
	if err != nil {
		return err
	}
	if err := bunnydns.CleanUpRecord(context.Background(), bunnyClient, ch.ResolvedFQDN, ch.ResolvedZone, ch.Key); err != nil {
		return err
	}
	log.Printf("CleanUp: TXT record removed/verified absent for %s in zone %s", ch.ResolvedFQDN, ch.ResolvedZone)
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
