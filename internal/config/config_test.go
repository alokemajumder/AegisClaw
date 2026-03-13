package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	// Override "7d" default which Go's time.ParseDuration doesn't support
	os.Setenv("AEGISCLAW_AUTH_REFRESH_EXPIRY", "168h")
	defer os.Unsetenv("AEGISCLAW_AUTH_REFRESH_EXPIRY")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.APIPort)
	assert.Equal(t, 9090, cfg.Server.GRPCBasePort)
	assert.Equal(t, "development", cfg.Server.Environment)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 25, cfg.Database.MaxConns)
	assert.Equal(t, "disable", cfg.Database.SSLMode)
	assert.Equal(t, false, cfg.TLS.Enabled)
}

func TestLoad_EnvOverride(t *testing.T) {
	os.Setenv("AEGISCLAW_SERVER_API_PORT", "9999")
	os.Setenv("AEGISCLAW_DATABASE_HOST", "db.example.com")
	os.Setenv("AEGISCLAW_AUTH_REFRESH_EXPIRY", "168h")
	defer os.Unsetenv("AEGISCLAW_SERVER_API_PORT")
	defer os.Unsetenv("AEGISCLAW_DATABASE_HOST")
	defer os.Unsetenv("AEGISCLAW_AUTH_REFRESH_EXPIRY")

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, 9999, cfg.Server.APIPort)
	assert.Equal(t, "db.example.com", cfg.Database.Host)
}

func TestValidate_ValidDevelopment(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "development"},
		Database: DatabaseConfig{Host: "localhost", MaxConns: 25, SSLMode: "disable"},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: -1, GRPCBasePort: 9090, Environment: "development"},
		Database: DatabaseConfig{Host: "localhost", MaxConns: 25},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_port")
}

func TestValidate_InvalidMaxConns(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "development"},
		Database: DatabaseConfig{Host: "localhost", MaxConns: 0},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max_conns")
}

func TestValidate_EmptyHost(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "development"},
		Database: DatabaseConfig{Host: "", MaxConns: 25},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

func TestValidate_ProductionDefaultJWT(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db", MaxConns: 25, SSLMode: "require", Password: "strongpwd"},
		Auth:     AuthConfig{JWTSecret: "dev-secret-change-in-production"},
		MinIO:    MinIOConfig{UseSSL: true, SecretKey: "changedkey"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jwt_secret must be set to a strong secret")
}

func TestValidate_ProductionShortJWT(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db", MaxConns: 25, SSLMode: "require", Password: "strongpwd"},
		Auth:     AuthConfig{JWTSecret: "short"},
		MinIO:    MinIOConfig{UseSSL: true, SecretKey: "changedkey"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least 32 characters")
}

func TestValidate_ProductionSSLDisabled(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db", MaxConns: 25, SSLMode: "disable", Password: "strongpwd"},
		Auth:     AuthConfig{JWTSecret: "a-very-long-secret-that-is-at-least-32-chars"},
		MinIO:    MinIOConfig{UseSSL: true, SecretKey: "changedkey"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssl_mode")
}

func TestValidate_ProductionDefaultPassword(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db", MaxConns: 25, SSLMode: "require", Password: "aegisclaw"},
		Auth:     AuthConfig{JWTSecret: "a-very-long-secret-that-is-at-least-32-chars"},
		MinIO:    MinIOConfig{UseSSL: true, SecretKey: "changedkey"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database.password must be changed")
}

func TestValidate_ProductionDefaultMinIO(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db", MaxConns: 25, SSLMode: "require", Password: "strongpwd"},
		Auth:     AuthConfig{JWTSecret: "a-very-long-secret-that-is-at-least-32-chars"},
		MinIO:    MinIOConfig{UseSSL: true, SecretKey: "minioadmin"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minio.secret_key")
}

func TestValidate_ProductionMinIONoSSL(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db", MaxConns: 25, SSLMode: "require", Password: "strongpwd"},
		Auth:     AuthConfig{JWTSecret: "a-very-long-secret-that-is-at-least-32-chars"},
		MinIO:    MinIOConfig{UseSSL: false, SecretKey: "changedkey"},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "minio.use_ssl")
}

func TestValidate_ProductionAllValid(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: 8080, GRPCBasePort: 9090, Environment: "production"},
		Database: DatabaseConfig{Host: "db.prod", MaxConns: 50, SSLMode: "require", Password: "strong-prod-password"},
		Auth:     AuthConfig{JWTSecret: "a-very-long-secret-that-is-at-least-32-chars", ReceiptHMACKey: "a-strong-receipt-hmac-key-for-prod"},
		MinIO:    MinIOConfig{UseSSL: true, SecretKey: "prod-minio-secret-key"},
	}
	assert.NoError(t, cfg.Validate())
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{APIPort: -1, GRPCBasePort: 99999, Environment: "production"},
		Database: DatabaseConfig{Host: "", MaxConns: 0},
		Auth:     AuthConfig{JWTSecret: ""},
	}
	err := cfg.Validate()
	assert.Error(t, err)
	// Should contain multiple error lines
	assert.Contains(t, err.Error(), "api_port")
	assert.Contains(t, err.Error(), "grpc_base_port")
	assert.Contains(t, err.Error(), "max_conns")
	assert.Contains(t, err.Error(), "host is required")
}

func TestDSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host: "localhost", Port: 5432, User: "user", Password: "pass", Name: "db", SSLMode: "disable",
	}
	assert.Equal(t, "postgres://user:pass@localhost:5432/db?sslmode=disable", cfg.DSN())
}
