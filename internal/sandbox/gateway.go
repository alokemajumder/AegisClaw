package sandbox

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// GatewayConfig holds connection settings for the OpenShell Gateway.
// The gateway runs on a single port (default 9090) multiplexing gRPC and HTTP.
type GatewayConfig struct {
	URL      string `mapstructure:"url"`       // Gateway endpoint (e.g. https://localhost:9090)
	AuthMode string `mapstructure:"auth_mode"` // "mtls" (default), "token", "none"
	CertFile string `mapstructure:"cert_file"` // Client certificate path (mTLS)
	KeyFile  string `mapstructure:"key_file"`  // Client private key path (mTLS)
	CAFile   string `mapstructure:"ca_file"`   // CA certificate path (mTLS)
	Token    string `mapstructure:"token"`     // Bearer token (token auth mode)
}

// GatewayClient communicates with the OpenShell control-plane API.
// It manages sandbox lifecycle, distributes policies, and configures inference routing.
type GatewayClient struct {
	httpClient *http.Client
	baseURL    string
	authMode   string
	token      string
	logger     *slog.Logger
}

// OpenShellPolicy defines the full sandbox security policy.
// Filesystem and process policies are static (set at creation).
// Network policies are hot-reloadable via the policy API.
type OpenShellPolicy struct {
	Version          int                      `yaml:"version" json:"version"`
	FilesystemPolicy FilesystemPolicy         `yaml:"filesystem_policy" json:"filesystem_policy"`
	Landlock         LandlockPolicy           `yaml:"landlock" json:"landlock"`
	Process          ProcessPolicy            `yaml:"process" json:"process"`
	NetworkPolicies  map[string]NetworkPolicy `yaml:"network_policies" json:"network_policies"`
}

// FilesystemPolicy controls sandbox filesystem access.
type FilesystemPolicy struct {
	IncludeWorkdir bool     `yaml:"include_workdir" json:"include_workdir"`
	ReadOnly       []string `yaml:"read_only" json:"read_only"`
	ReadWrite      []string `yaml:"read_write" json:"read_write"`
}

// LandlockPolicy controls Landlock LSM enforcement behavior.
type LandlockPolicy struct {
	Compatibility string `yaml:"compatibility" json:"compatibility"` // best_effort or hard_requirement
}

// ProcessPolicy sets the user and group the sandbox process runs as.
type ProcessPolicy struct {
	RunAsUser  string `yaml:"run_as_user" json:"run_as_user"`
	RunAsGroup string `yaml:"run_as_group" json:"run_as_group"`
}

// NetworkPolicy defines a named network access policy with endpoint rules.
type NetworkPolicy struct {
	Name      string            `yaml:"name" json:"name"`
	Endpoints []NetworkEndpoint `yaml:"endpoints" json:"endpoints"`
	Binaries  []BinaryRule      `yaml:"binaries,omitempty" json:"binaries,omitempty"`
}

// NetworkEndpoint defines a single allowed or denied network destination.
type NetworkEndpoint struct {
	Host        string `yaml:"host" json:"host"`
	Port        int    `yaml:"port" json:"port"`
	Protocol    string `yaml:"protocol" json:"protocol"`
	TLS         string `yaml:"tls,omitempty" json:"tls,omitempty"`
	Enforcement string `yaml:"enforcement" json:"enforcement"` // enforce, audit
	Access      string `yaml:"access,omitempty" json:"access,omitempty"`
}

// BinaryRule restricts which binaries may use a network policy.
type BinaryRule struct {
	Path string `yaml:"path" json:"path"`
}

// CreateSandboxRequest is the payload for creating a new sandbox.
type CreateSandboxRequest struct {
	Name   string            `json:"name"`
	Image  string            `json:"image"` // base, ollama, openclaw
	Policy *OpenShellPolicy  `json:"policy,omitempty"`
	GPU    bool              `json:"gpu,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

// GatewaySandbox represents a running or stopped sandbox instance.
type GatewaySandbox struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Image     string            `json:"image"`
	CreatedAt time.Time         `json:"created_at"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// GatewayStatus is the health response from the gateway.
type GatewayStatus struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Sandboxes int    `json:"sandboxes"`
}

// InferenceConfig describes the inference provider routing configuration.
// Sandboxes reach inference via the Privacy Router at https://inference.local/v1/...
type InferenceConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Type     string `json:"type"` // nvidia, openai, anthropic
}

// gatewayError represents an error response from the gateway API.
type gatewayError struct {
	StatusCode int
	Message    string
}

func (e *gatewayError) Error() string {
	return fmt.Sprintf("gateway responded %d: %s", e.StatusCode, e.Message)
}

// NewGatewayClient creates a GatewayClient configured for the given gateway.
// For mTLS auth (the default), it loads client certificates and the CA from disk.
func NewGatewayClient(ctx context.Context, cfg GatewayConfig, logger *slog.Logger) (*GatewayClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("creating gateway client: url is required")
	}
	if cfg.AuthMode == "" {
		cfg.AuthMode = "mtls"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	if cfg.AuthMode == "mtls" {
		tlsCfg, err := buildMTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("creating gateway client: %w", err)
		}
		transport.TLSClientConfig = tlsCfg
		logger.Info("gateway client configured with mTLS",
			"url", cfg.URL,
			"cert", cfg.CertFile,
			"ca", cfg.CAFile,
		)
	} else {
		logger.Info("gateway client configured",
			"url", cfg.URL,
			"auth_mode", cfg.AuthMode,
		)
	}

	return &GatewayClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
		baseURL:  cfg.URL,
		authMode: cfg.AuthMode,
		token:    cfg.Token,
		logger:   logger,
	}, nil
}

// buildMTLSConfig loads client cert, key, and CA to produce a tls.Config.
func buildMTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, fmt.Errorf("building mTLS config: cert_file, key_file, and ca_file are all required")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("parsing CA certificate: no valid certificates found in %s", caFile)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ---------------------------------------------------------------------------
// Sandbox lifecycle
// ---------------------------------------------------------------------------

// CreateSandbox creates a new sandbox from the given request.
// Filesystem and process policies are fixed at creation time.
func (c *GatewayClient) CreateSandbox(ctx context.Context, req CreateSandboxRequest) (*GatewaySandbox, error) {
	c.logger.Info("creating sandbox", "name", req.Name, "image", req.Image, "gpu", req.GPU)

	var sb GatewaySandbox
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes", req, &sb); err != nil {
		return nil, fmt.Errorf("creating sandbox %q: %w", req.Name, err)
	}
	return &sb, nil
}

// GetSandbox returns the current state of a sandbox.
func (c *GatewayClient) GetSandbox(ctx context.Context, name string) (*GatewaySandbox, error) {
	var sb GatewaySandbox
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sandboxes/"+url.PathEscape(name), nil, &sb); err != nil {
		return nil, fmt.Errorf("getting sandbox %q: %w", name, err)
	}
	return &sb, nil
}

// ListSandboxes returns all sandboxes known to the gateway.
func (c *GatewayClient) ListSandboxes(ctx context.Context) ([]GatewaySandbox, error) {
	var sandboxes []GatewaySandbox
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sandboxes", nil, &sandboxes); err != nil {
		return nil, fmt.Errorf("listing sandboxes: %w", err)
	}
	return sandboxes, nil
}

// DeleteSandbox tears down a sandbox and releases its resources.
func (c *GatewayClient) DeleteSandbox(ctx context.Context, name string) error {
	c.logger.Info("deleting sandbox", "name", name)
	if err := c.doJSON(ctx, http.MethodDelete, "/v1/sandboxes/"+url.PathEscape(name), nil, nil); err != nil {
		return fmt.Errorf("deleting sandbox %q: %w", name, err)
	}
	return nil
}

// UploadFile uploads a local file into a sandbox at the given remote path.
func (c *GatewayClient) UploadFile(ctx context.Context, name, localPath, remotePath string) error {
	c.logger.Info("uploading file to sandbox", "sandbox", name, "local", localPath, "remote", remotePath)

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("uploading file to sandbox %q: opening %s: %w", name, localPath, err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("remote_path", remotePath); err != nil {
		return fmt.Errorf("uploading file to sandbox %q: writing remote_path field: %w", name, err)
	}

	part, err := writer.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return fmt.Errorf("uploading file to sandbox %q: creating form file: %w", name, err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("uploading file to sandbox %q: copying file data: %w", name, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("uploading file to sandbox %q: closing multipart writer: %w", name, err)
	}

	endpoint := fmt.Sprintf("/v1/sandboxes/%s/upload", url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, &body)
	if err != nil {
		return fmt.Errorf("uploading file to sandbox %q: building request: %w", name, err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("uploading file to sandbox %q: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return c.readError(resp)
	}
	return nil
}

// DownloadFile downloads a file from a sandbox to a local path.
func (c *GatewayClient) DownloadFile(ctx context.Context, name, remotePath, localPath string) error {
	c.logger.Info("downloading file from sandbox", "sandbox", name, "remote", remotePath, "local", localPath)

	endpoint := fmt.Sprintf("/v1/sandboxes/%s/download?path=%s",
		url.PathEscape(name), url.QueryEscape(remotePath))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("downloading file from sandbox %q: building request: %w", name, err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading file from sandbox %q: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return c.readError(resp)
	}

	out, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("downloading file from sandbox %q: creating %s: %w", name, localPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("downloading file from sandbox %q: writing %s: %w", name, localPath, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Policy management
// ---------------------------------------------------------------------------

// GetPolicy returns a named policy attached to a sandbox.
func (c *GatewayClient) GetPolicy(ctx context.Context, sandboxName, policyName string) (*OpenShellPolicy, error) {
	endpoint := fmt.Sprintf("/v1/sandboxes/%s/policies/%s",
		url.PathEscape(sandboxName), url.PathEscape(policyName))

	var policy OpenShellPolicy
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &policy); err != nil {
		return nil, fmt.Errorf("getting policy %q for sandbox %q: %w", policyName, sandboxName, err)
	}
	return &policy, nil
}

// SetPolicy creates or updates a named policy on a sandbox.
// Network policies are hot-reloadable; filesystem and process policies
// require sandbox recreation to take effect.
func (c *GatewayClient) SetPolicy(ctx context.Context, sandboxName, policyName string, policy OpenShellPolicy) error {
	c.logger.Info("setting policy on sandbox", "sandbox", sandboxName, "policy", policyName)

	endpoint := fmt.Sprintf("/v1/sandboxes/%s/policies/%s",
		url.PathEscape(sandboxName), url.PathEscape(policyName))

	if err := c.doJSON(ctx, http.MethodPut, endpoint, policy, nil); err != nil {
		return fmt.Errorf("setting policy %q for sandbox %q: %w", policyName, sandboxName, err)
	}
	return nil
}

// ListPolicies returns all policies attached to a sandbox.
func (c *GatewayClient) ListPolicies(ctx context.Context, sandboxName string) ([]OpenShellPolicy, error) {
	endpoint := fmt.Sprintf("/v1/sandboxes/%s/policies", url.PathEscape(sandboxName))

	var policies []OpenShellPolicy
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &policies); err != nil {
		return nil, fmt.Errorf("listing policies for sandbox %q: %w", sandboxName, err)
	}
	return policies, nil
}

// ---------------------------------------------------------------------------
// Inference routing
// ---------------------------------------------------------------------------

// SetInferenceProvider configures the Privacy Router's upstream inference provider.
// Sandboxes reach inference at https://inference.local/v1/... which the gateway
// routes to the configured provider (nvidia, openai, anthropic).
func (c *GatewayClient) SetInferenceProvider(ctx context.Context, providerType string, cfg InferenceConfig) error {
	c.logger.Info("setting inference provider", "type", providerType, "model", cfg.Model)

	cfg.Type = providerType
	if err := c.doJSON(ctx, http.MethodPut, "/v1/inference", cfg, nil); err != nil {
		return fmt.Errorf("setting inference provider: %w", err)
	}
	return nil
}

// GetInferenceConfig returns the current inference routing configuration.
func (c *GatewayClient) GetInferenceConfig(ctx context.Context) (*InferenceConfig, error) {
	var cfg InferenceConfig
	if err := c.doJSON(ctx, http.MethodGet, "/v1/inference", nil, &cfg); err != nil {
		return nil, fmt.Errorf("getting inference config: %w", err)
	}
	return &cfg, nil
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// GatewayHealth returns the gateway's health status including sandbox count.
func (c *GatewayClient) GatewayHealth(ctx context.Context) (*GatewayStatus, error) {
	var status GatewayStatus
	if err := c.doJSON(ctx, http.MethodGet, "/v1/health", nil, &status); err != nil {
		return nil, fmt.Errorf("checking gateway health: %w", err)
	}
	return &status, nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// doJSON performs an HTTP request with optional JSON body and decodes the response.
func (c *GatewayClient) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return c.readError(resp)
	}

	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

// setAuth applies the configured authentication to an outgoing request.
// For mTLS, the TLS client certificate on the transport handles auth.
// For token mode, a Bearer header is added.
func (c *GatewayClient) setAuth(req *http.Request) {
	if c.authMode == "token" && c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	// mTLS auth is handled by the TLS transport — no header needed.
}

// readError extracts an error message from a non-2xx gateway response.
func (c *GatewayClient) readError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	// Try to extract a structured error message.
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		msg := errResp.Error
		if msg == "" {
			msg = errResp.Message
		}
		if msg != "" {
			return &gatewayError{StatusCode: resp.StatusCode, Message: msg}
		}
	}

	// Fall back to the raw body.
	msg := string(body)
	if msg == "" {
		msg = resp.Status
	}
	return &gatewayError{StatusCode: resp.StatusCode, Message: msg}
}
