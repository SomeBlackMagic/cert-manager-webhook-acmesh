package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
	"github.com/jetstack/cert-manager/pkg/issuer/acme/dns/util"
)

const (
	defaultTTL        = 600
	acmeDelegate      = "/acmebin/acme_delegate.sh"
	acmeDelegateTimeout = 2 * time.Minute
)

var (
// version and revision are set at build time via -ldflags.
	version  = "dev"
	revision = "unknown"
	GroupName    = os.Getenv("GROUP_NAME")
	validDNSAPI  = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	retvalRe     = regexp.MustCompile(`ACME_RETVAL(\d+)ACME_RETVAL`)
)

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	klog.Infof("Starting webhook server with group name: %s with version: %s(%s)", GroupName, version, revision)
	cmd.RunWebhookServer(GroupName, &customDNSProviderSolver{})
}

type customDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type envSecretRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type customDNSProviderConfig struct {
	TTL          *uint64      `json:"ttl"`
	DNSAPI       string       `json:"dnsapi"`
	EnvSecretRef envSecretRef `json:"env"`
	Debug        *int         `json:"debug"`
}

func (c *customDNSProviderSolver) Name() string {
	return "acmesh"
}

func (c *customDNSProviderSolver) DoDNSAPI(action string, ch *v1alpha1.ChallengeRequest) error {
	klog.Infof("Starting DoDNSAPI action=%s domain=%s", action, ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		klog.Errorf("Failed to parse config: %v", err)
		return err
	}

	if !validDNSAPI.MatchString(cfg.DNSAPI) {
		return fmt.Errorf("invalid dnsapi name %q: must match ^[a-zA-Z0-9_]+$", cfg.DNSAPI)
	}

	klog.Infof("Loading secret %s/%s", cfg.EnvSecretRef.Namespace, cfg.EnvSecretRef.Name)
	envSecret, err := c.client.CoreV1().Secrets(cfg.EnvSecretRef.Namespace).Get(context.TODO(), cfg.EnvSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get secret %s/%s: %v", cfg.EnvSecretRef.Namespace, cfg.EnvSecretRef.Name, err)
		return err
	}

	envVars := []string{}
	for key, val := range envSecret.Data {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, string(val)))
	}
	if cfg.Debug != nil {
		envVars = append(envVars, fmt.Sprintf("DEBUG=%d", *cfg.Debug))
		klog.Infof("acme_delegate debug level set to %d", *cfg.Debug)
	}

	ctx, cancel := context.WithTimeout(context.Background(), acmeDelegateTimeout)
	defer cancel()

	klog.Infof("Executing %s dnsapi=%s action=%s domain=%s", acmeDelegate, cfg.DNSAPI, action, util.UnFqdn(ch.ResolvedFQDN))
	cmd := exec.CommandContext(ctx, acmeDelegate, cfg.DNSAPI, action, util.UnFqdn(ch.ResolvedFQDN), ch.Key)
	cmd.Env = envVars

	out, err := cmd.CombinedOutput()
	klog.Infof("acme_delegate output (%d bytes):\n%s", len(out), string(out))

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("acme_delegate timed out after %s", acmeDelegateTimeout)
	}

	matches := retvalRe.FindStringSubmatch(string(out))
	if len(matches) < 2 {
		return fmt.Errorf("acme_delegate output did not contain ACME_RETVAL marker; err=%v", err)
	}

	retval := matches[1]
	if retval != "0" {
		klog.Warningf("acme_delegate failed for domain=%s retval=%s", ch.ResolvedFQDN, retval)
		return fmt.Errorf("acme_delegate failed with retval=%s", retval)
	}

	klog.Infof("acme_delegate succeeded for domain=%s", ch.ResolvedFQDN)
	return nil
}

func (c *customDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	klog.Infof("Presenting DNS challenge for %s", ch.ResolvedFQDN)
	return c.DoDNSAPI("add", ch)
}

func (c *customDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	klog.Infof("Cleaning up DNS challenge for %s", ch.ResolvedFQDN)
	return c.DoDNSAPI("rm", ch)
}

func (c *customDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	klog.Infof("Initializing Kubernetes client")
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		klog.Errorf("Failed to create Kubernetes client: %v", err)
		return err
	}
	c.client = cl
	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (customDNSProviderConfig, error) {
	ttl := uint64(defaultTTL)
	cfg := customDNSProviderConfig{TTL: &ttl}
	if cfgJSON == nil {
		klog.Infof("No config JSON provided; using default TTL=%d", defaultTTL)
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		klog.Errorf("Failed to unmarshal solver config: %v", err)
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}
	klog.Infof("Loaded config: DNSAPI=%s TTL=%d", cfg.DNSAPI, *cfg.TTL)
	return cfg, nil
}
