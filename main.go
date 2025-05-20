package main

import (
	"context"
	"encoding/json"
	"fmt"
	"k8s.io/client-go/kubernetes"
	"os"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
	"github.com/jetstack/cert-manager/pkg/issuer/acme/dns/util"

	"github.com/google/uuid"
)

const (
	defaultTTL      = 600
	acmeDelegate    = "/acme_delegate.sh"
	acmeReturnValue = "ACME_RETVAL"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	klog.Infof("Starting webhook server with group name: %s", GroupName)
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
}

type envFromSecret []string

func (c *customDNSProviderSolver) Name() string {
	return "acmesh"
}

func (c *customDNSProviderSolver) DoDNSAPI(action string, ch *v1alpha1.ChallengeRequest) error {
	klog.Infof("Starting DoDNSAPI with action=%s for domain=%s", action, ch.ResolvedFQDN)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		klog.Errorf("Failed to parse config: %v", err)
		return err
	}

	klog.Infof("Loading secret %s/%s", cfg.EnvSecretRef.Namespace, cfg.EnvSecretRef.Name)
	envSecret, err := c.client.CoreV1().Secrets(cfg.EnvSecretRef.Namespace).Get(context.TODO(), cfg.EnvSecretRef.Name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get secret: %v", err)
		return err
	}

	envData, ok := envSecret.Data["env"]
	if !ok {
		klog.Errorf("Missing 'env' key in secret")
		return fmt.Errorf("no env in secret")
	}

	env := envFromSecret{}
	if err := json.Unmarshal(envData, &env); err != nil {
		klog.Errorf("Failed to unmarshal env data: %v", err)
		return err
	}

	uuid := uuid.New()
	stdoutFile, err := os.CreateTemp("/tmp", uuid.String())
	defer os.Remove(stdoutFile.Name())
	if err != nil {
		klog.Errorf("Failed to create temp file: %v", err)
		return err
	}

	procAttr := &os.ProcAttr{
		Files: []*os.File{os.Stdin, stdoutFile, os.Stderr},
		Env:   env,
	}

	dir, err := os.Getwd()
	if err != nil {
		klog.Errorf("Failed to get working directory: %v", err)
		return err
	}

	klog.Infof("Executing %s with action=%s", acmeDelegate, action)
	process, err := os.StartProcess(dir+acmeDelegate, []string{
		dir + acmeDelegate, cfg.DNSAPI, action, util.UnFqdn(ch.ResolvedFQDN), ch.Key,
	}, procAttr)
	if err != nil {
		klog.Errorf("Failed to start process: %v", err)
		return err
	}

	process.Wait()
	stdoutFile.Sync()

	outFile, err := os.Open(stdoutFile.Name())
	if err != nil {
		klog.Errorf("Failed to read output file: %v", err)
		return err
	}

	output := make([]byte, 1048576)
	count, err := outFile.Read(output)
	if err != nil {
		klog.Errorf("Failed to read output content: %v", err)
		return err
	}

	klog.Infof("Process output (%d bytes): %s", count, string(output))
	lines := strings.Split(string(output), "\n")

	retval := "0"
	for _, line := range lines {
		if strings.HasPrefix(line, acmeReturnValue) {
			items := strings.Split(line, acmeReturnValue)
			retval = items[1]
		}
	}

	if retval == "0" {
		klog.Infof("ACME script succeeded for domain=%s", ch.ResolvedFQDN)
		return nil
	}

	klog.Warningf("ACME script returned failure: %s", retval)
	return fmt.Errorf("failed to run acme.sh, error=%s ", retval)
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
	klog.Infof("Loaded config: DNSAPI=%s, TTL=%d", cfg.DNSAPI, *cfg.TTL)
	return cfg, nil
}
