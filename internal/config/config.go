package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all platform configuration.
type Config struct {
	Server        ServerConfig         `mapstructure:"server"`
	Database      DatabaseConfig       `mapstructure:"database"`
	NATS          NATSConfig           `mapstructure:"nats"`
	MinIO         MinIOConfig          `mapstructure:"minio"`
	Ollama        OllamaConfig         `mapstructure:"ollama"`
	NVIDIANIMM    NVIDIANIMConfig      `mapstructure:"nvidia_nim"`
	Guardrails    NeMoGuardrailsConfig `mapstructure:"nemo_guardrails"`
	Sandbox       SandboxConfig        `mapstructure:"sandbox"`
	Auth          AuthConfig           `mapstructure:"auth"`
	Policy        PolicyConfig         `mapstructure:"policy"`
	Observability ObservabilityConfig  `mapstructure:"observability"`
	TLS           TLSConfig            `mapstructure:"tls"`
}

// SandboxConfig holds NemoClaw/OpenShell sandbox configuration.
// When enabled, agent execution steps run inside Landlock+seccomp+netns sandboxes
// matching the governance tier policy via the OpenShell Gateway.
type SandboxConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	RuntimeURL     string `mapstructure:"runtime_url"`     // OpenShell Gateway endpoint (e.g. https://localhost:9090)
	PolicyDir      string `mapstructure:"policy_dir"`      // Directory containing .yaml policy files
	TimeoutSeconds int    `mapstructure:"timeout_seconds"` // Per-step execution timeout
	MaxMemoryMB    int    `mapstructure:"max_memory_mb"`   // Memory limit per sandbox
	MaxCPUCores    int    `mapstructure:"max_cpu_cores"`   // CPU core limit per sandbox
	NetworkPolicy  string `mapstructure:"network_policy"`  // Default network policy (deny_all, allow_connectors, allow_all)
	Image          string `mapstructure:"image"`           // Sandbox base image (base, ollama, openclaw)
	GPU            bool   `mapstructure:"gpu"`             // Enable GPU passthrough for inference in sandboxes
	AuthMode       string `mapstructure:"auth_mode"`       // Gateway auth: mtls, token, none
	CertFile       string `mapstructure:"cert_file"`       // Client cert for mTLS
	KeyFile        string `mapstructure:"key_file"`        // Client key for mTLS
	CAFile         string `mapstructure:"ca_file"`         // CA cert for mTLS
	GatewayToken   string `mapstructure:"gateway_token"`   // Bearer token for token auth
}

type ServerConfig struct {
	APIPort      int      `mapstructure:"api_port"`
	GRPCBasePort int      `mapstructure:"grpc_base_port"`
	CORSOrigins  []string `mapstructure:"cors_origins"`
	Environment  string   `mapstructure:"environment"`
	PlaybookDir  string   `mapstructure:"playbook_dir"`
}

// TLSConfig holds TLS certificate configuration.
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"ssl_mode"`
	MaxConns int    `mapstructure:"max_conns"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
	)
}

type NATSConfig struct {
	URL            string        `mapstructure:"url"`
	MaxReconnects  int           `mapstructure:"max_reconnects"`
	ReconnectWait  time.Duration `mapstructure:"reconnect_wait"`
}

type MinIOConfig struct {
	Endpoint  string `mapstructure:"endpoint"`
	AccessKey string `mapstructure:"access_key"`
	SecretKey string `mapstructure:"secret_key"`
	Bucket    string `mapstructure:"bucket"`
	UseSSL    bool   `mapstructure:"use_ssl"`
}

type OllamaConfig struct {
	URL            string   `mapstructure:"url"`
	DefaultModel   string   `mapstructure:"default_model"`
	ModelAllowlist []string `mapstructure:"model_allowlist"`
	TimeoutSeconds int      `mapstructure:"timeout_seconds"`
}

// NVIDIANIMConfig holds NVIDIA NIM / NeMoClaw configuration.
// When enabled, NIM is used as the primary LLM backend (with Ollama as fallback).
// Supports NVIDIA API Catalog (build.nvidia.com), self-hosted NIM on DGX/RTX,
// consumer GPUs (RTX 4090/5090), and NemoClaw always-on assistants.
type NVIDIANIMConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	URL            string   `mapstructure:"url"`              // NIM API endpoint (e.g. https://integrate.api.nvidia.com/v1)
	APIKey         string   `mapstructure:"api_key"`          // NVIDIA API key (required for cloud, optional for self-hosted)
	APIKeyRef      string   `mapstructure:"api_key_ref"`      // Environment variable holding the API key
	DefaultModel   string   `mapstructure:"default_model"`    // e.g. nvidia/nemotron-3-super-120b-a12b
	ModelAllowlist []string `mapstructure:"model_allowlist"`  // Allowed NIM models
	TimeoutSeconds int      `mapstructure:"timeout_seconds"`
	ThinkingBudget int      `mapstructure:"thinking_budget"`  // Reasoning depth 0-10 (0=default, higher=deeper/slower)
}

// NeMoGuardrailsConfig holds NeMo Guardrails NIM configuration.
// Provides content safety, jailbreak detection, and topic control for LLM prompts.
type NeMoGuardrailsConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	ContentSafetyURL string `mapstructure:"content_safety_url"` // Content Safety NIM endpoint
	JailbreakURL     string `mapstructure:"jailbreak_url"`      // Jailbreak Detection NIM endpoint
	TopicControlURL  string `mapstructure:"topic_control_url"`  // Topic Control NIM endpoint
	TimeoutSeconds   int    `mapstructure:"timeout_seconds"`
}

type AuthConfig struct {
	JWTSecret       string        `mapstructure:"jwt_secret"`
	JWTSecretRef    string        `mapstructure:"jwt_secret_ref"`
	TokenExpiry     time.Duration `mapstructure:"token_expiry"`
	RefreshExpiry   time.Duration `mapstructure:"refresh_expiry"`
	ReceiptHMACKey  string        `mapstructure:"receipt_hmac_key"`
	SSO             SSOConfig     `mapstructure:"sso"`
}

type SSOConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Provider  string `mapstructure:"provider"`
	IssuerURL string `mapstructure:"issuer_url"`
	ClientID  string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
}

type PolicyConfig struct {
	DefaultPack        string `mapstructure:"default_pack"`
	GlobalRateLimit    int    `mapstructure:"global_rate_limit"`
	GlobalConcurrencyCap int  `mapstructure:"global_concurrency_cap"`
}

type ObservabilityConfig struct {
	TracingEndpoint string  `mapstructure:"tracing_endpoint"`
	MetricsPort     int     `mapstructure:"metrics_port"`
	LogLevel        string  `mapstructure:"log_level"`
	SamplingRate    float64 `mapstructure:"sampling_rate"`
}

// Load reads configuration from file and environment variables.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.api_port", 8080)
	v.SetDefault("server.grpc_base_port", 9090)
	v.SetDefault("server.cors_origins", []string{"http://localhost:3000"})
	v.SetDefault("server.environment", "development")
	v.SetDefault("server.playbook_dir", "./playbooks")
	v.SetDefault("tls.enabled", false)
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "aegisclaw")
	v.SetDefault("database.password", "aegisclaw")
	v.SetDefault("database.name", "aegisclaw")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_conns", 25)
	v.SetDefault("nats.url", "nats://localhost:4222")
	v.SetDefault("nats.max_reconnects", 60)
	v.SetDefault("nats.reconnect_wait", "2s")
	v.SetDefault("minio.endpoint", "localhost:9000")
	v.SetDefault("minio.access_key", "minioadmin")
	v.SetDefault("minio.secret_key", "minioadmin")
	v.SetDefault("minio.bucket", "aegisclaw-evidence")
	v.SetDefault("minio.use_ssl", false)
	v.SetDefault("ollama.url", "http://localhost:11434")
	v.SetDefault("ollama.default_model", "llama3.1")
	v.SetDefault("ollama.timeout_seconds", 120)
	v.SetDefault("nvidia_nim.enabled", false)
	v.SetDefault("nvidia_nim.url", "https://integrate.api.nvidia.com/v1")
	v.SetDefault("nvidia_nim.default_model", "nvidia/nemotron-3-super-120b-a12b")
	v.SetDefault("nvidia_nim.timeout_seconds", 120)
	v.SetDefault("nvidia_nim.thinking_budget", 0)
	v.SetDefault("nemo_guardrails.enabled", false)
	v.SetDefault("nemo_guardrails.content_safety_url", "http://localhost:8180/v1")
	v.SetDefault("nemo_guardrails.jailbreak_url", "http://localhost:8181/v1")
	v.SetDefault("nemo_guardrails.topic_control_url", "http://localhost:8182/v1")
	v.SetDefault("nemo_guardrails.timeout_seconds", 10)
	v.SetDefault("sandbox.enabled", false)
	v.SetDefault("sandbox.runtime_url", "http://localhost:8765")
	v.SetDefault("sandbox.policy_dir", "./configs/sandbox-policies")
	v.SetDefault("sandbox.timeout_seconds", 300)
	v.SetDefault("sandbox.max_memory_mb", 2048)
	v.SetDefault("sandbox.max_cpu_cores", 2)
	v.SetDefault("sandbox.network_policy", "deny_all")
	v.SetDefault("sandbox.image", "base")
	v.SetDefault("sandbox.gpu", false)
	v.SetDefault("sandbox.auth_mode", "none")
	v.SetDefault("auth.token_expiry", "15m")
	v.SetDefault("auth.refresh_expiry", "7d")
	v.SetDefault("auth.receipt_hmac_key", "dev-receipt-key-change-in-production")
	v.SetDefault("policy.default_pack", "default")
	v.SetDefault("policy.global_rate_limit", 100)
	v.SetDefault("policy.global_concurrency_cap", 20)
	v.SetDefault("observability.tracing_endpoint", "http://localhost:4317")
	v.SetDefault("observability.metrics_port", 9100)
	v.SetDefault("observability.log_level", "info")
	v.SetDefault("observability.sampling_rate", 1.0)

	// Config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("aegisclaw")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/aegisclaw")
	}

	// Environment variables: AEGISCLAW_DATABASE_HOST -> database.host
	v.SetEnvPrefix("AEGISCLAW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		// Config file not found is OK — use defaults + env vars
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks configuration for production readiness.
func (c *Config) Validate() error {
	var errs []string

	if c.Server.APIPort <= 0 || c.Server.APIPort > 65535 {
		errs = append(errs, "server.api_port must be between 1 and 65535")
	}
	if c.Server.GRPCBasePort <= 0 || c.Server.GRPCBasePort > 65535 {
		errs = append(errs, "server.grpc_base_port must be between 1 and 65535")
	}
	if c.Database.MaxConns <= 0 {
		errs = append(errs, "database.max_conns must be positive")
	}
	if c.Database.Host == "" {
		errs = append(errs, "database.host is required")
	}

	if c.Server.Environment == "production" {
		if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == "dev-secret-change-in-production" {
			errs = append(errs, "auth.jwt_secret must be set to a strong secret in production")
		}
		if len(c.Auth.JWTSecret) < 32 {
			errs = append(errs, "auth.jwt_secret must be at least 32 characters in production")
		}
		if c.Database.SSLMode == "disable" {
			errs = append(errs, "database.ssl_mode must not be 'disable' in production")
		}
		if c.Database.Password == "aegisclaw" {
			errs = append(errs, "database.password must be changed from default in production")
		}
		if c.MinIO.SecretKey == "minioadmin" {
			errs = append(errs, "minio.secret_key must be changed from default in production")
		}
		if !c.MinIO.UseSSL {
			errs = append(errs, "minio.use_ssl should be enabled in production")
		}
		if c.Auth.ReceiptHMACKey == "" || c.Auth.ReceiptHMACKey == "dev-receipt-key-change-in-production" {
			errs = append(errs, "auth.receipt_hmac_key must be set to a strong secret in production")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
