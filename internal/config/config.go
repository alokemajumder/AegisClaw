package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all platform configuration.
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	NATS          NATSConfig          `mapstructure:"nats"`
	MinIO         MinIOConfig         `mapstructure:"minio"`
	Ollama        OllamaConfig        `mapstructure:"ollama"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Policy        PolicyConfig        `mapstructure:"policy"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	TLS           TLSConfig           `mapstructure:"tls"`
}

type ServerConfig struct {
	APIPort     int      `mapstructure:"api_port"`
	GRPCBasePort int     `mapstructure:"grpc_base_port"`
	CORSOrigins []string `mapstructure:"cors_origins"`
	Environment string   `mapstructure:"environment"`
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

type AuthConfig struct {
	JWTSecret       string    `mapstructure:"jwt_secret"`
	JWTSecretRef    string    `mapstructure:"jwt_secret_ref"`
	TokenExpiry     time.Duration `mapstructure:"token_expiry"`
	RefreshExpiry   time.Duration `mapstructure:"refresh_expiry"`
	SSO             SSOConfig `mapstructure:"sso"`
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
	TracingEndpoint string `mapstructure:"tracing_endpoint"`
	MetricsPort     int    `mapstructure:"metrics_port"`
	LogLevel        string `mapstructure:"log_level"`
}

// Load reads configuration from file and environment variables.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.api_port", 8080)
	v.SetDefault("server.grpc_base_port", 9090)
	v.SetDefault("server.cors_origins", []string{"http://localhost:3000"})
	v.SetDefault("server.environment", "development")
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
	v.SetDefault("auth.token_expiry", "15m")
	v.SetDefault("auth.refresh_expiry", "7d")
	v.SetDefault("policy.default_pack", "default")
	v.SetDefault("policy.global_rate_limit", 100)
	v.SetDefault("policy.global_concurrency_cap", 20)
	v.SetDefault("observability.tracing_endpoint", "http://localhost:4317")
	v.SetDefault("observability.metrics_port", 9100)
	v.SetDefault("observability.log_level", "info")

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
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
