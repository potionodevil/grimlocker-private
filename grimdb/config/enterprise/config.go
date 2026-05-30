//go:build enterprise

package enterprise

import (
	"errors"
	"fmt"
	"os"
)

// EnterpriseConfig holds all environment-driven configuration for the Enterprise tier.
type EnterpriseConfig struct {
	// OIDC / IAM
	OIDCProviderURL  string // GRIMLOCKER_OIDC_PROVIDER
	OIDCClientID     string // GRIMLOCKER_OIDC_CLIENT_ID
	OIDCClientSecret string // GRIMLOCKER_OIDC_CLIENT_SECRET

	// S3 / MinIO storage
	VaultBackend string // GRIMLOCKER_VAULT_BACKEND: "s3" | "minio"
	S3Bucket     string // GRIMLOCKER_S3_BUCKET
	S3Region     string // GRIMLOCKER_S3_REGION
	S3Endpoint   string // GRIMLOCKER_S3_ENDPOINT (empty = AWS default)
	S3AccessKey  string // AWS_ACCESS_KEY_ID
	S3SecretKey  string // AWS_SECRET_ACCESS_KEY

	// mTLS certificates
	MTLSCertPath string // GRIMLOCKER_MTLS_CERT_PATH
	MTLSKeyPath  string // GRIMLOCKER_MTLS_KEY_PATH
	MTLSCAPath   string // GRIMLOCKER_MTLS_CA_PATH
	MTLSSPKIPin  string // GRIMLOCKER_MTLS_PIN_SPKI (optional public-key pin)

	// Runtime
	AppDir      string
	EntropyPath string
}

// LoadEnterpriseConfig reads and validates all enterprise environment variables.
// Returns an error if any required variable is missing or a cert file is unreadable.
func LoadEnterpriseConfig(appDir, entropyPath string) (*EnterpriseConfig, error) {
	cfg := &EnterpriseConfig{
		OIDCProviderURL:  os.Getenv("GRIMLOCKER_OIDC_PROVIDER"),
		OIDCClientID:     os.Getenv("GRIMLOCKER_OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("GRIMLOCKER_OIDC_CLIENT_SECRET"),
		VaultBackend:     envOrDefault("GRIMLOCKER_VAULT_BACKEND", "s3"),
		S3Bucket:         os.Getenv("GRIMLOCKER_S3_BUCKET"),
		S3Region:         envOrDefault("GRIMLOCKER_S3_REGION", "us-east-1"),
		S3Endpoint:       os.Getenv("GRIMLOCKER_S3_ENDPOINT"),
		S3AccessKey:      os.Getenv("AWS_ACCESS_KEY_ID"),
		S3SecretKey:      os.Getenv("AWS_SECRET_ACCESS_KEY"),
		MTLSCertPath:     os.Getenv("GRIMLOCKER_MTLS_CERT_PATH"),
		MTLSKeyPath:      os.Getenv("GRIMLOCKER_MTLS_KEY_PATH"),
		MTLSCAPath:       os.Getenv("GRIMLOCKER_MTLS_CA_PATH"),
		MTLSSPKIPin:      os.Getenv("GRIMLOCKER_MTLS_PIN_SPKI"),
		AppDir:           appDir,
		EntropyPath:      entropyPath,
	}

	var errs []error

	// Validate OIDC settings.
	if cfg.OIDCProviderURL == "" {
		errs = append(errs, errors.New("GRIMLOCKER_OIDC_PROVIDER is required"))
	}
	if cfg.OIDCClientID == "" {
		errs = append(errs, errors.New("GRIMLOCKER_OIDC_CLIENT_ID is required"))
	}

	// Validate S3 settings.
	if cfg.S3Bucket == "" {
		errs = append(errs, errors.New("GRIMLOCKER_S3_BUCKET is required"))
	}
	if cfg.S3AccessKey == "" {
		errs = append(errs, errors.New("AWS_ACCESS_KEY_ID is required"))
	}
	if cfg.S3SecretKey == "" {
		errs = append(errs, errors.New("AWS_SECRET_ACCESS_KEY is required"))
	}

	// Validate mTLS cert files exist (paths are required).
	for _, p := range []struct{ env, val string }{
		{"GRIMLOCKER_MTLS_CERT_PATH", cfg.MTLSCertPath},
		{"GRIMLOCKER_MTLS_KEY_PATH", cfg.MTLSKeyPath},
		{"GRIMLOCKER_MTLS_CA_PATH", cfg.MTLSCAPath},
	} {
		if p.val == "" {
			errs = append(errs, fmt.Errorf("%s is required", p.env))
		} else if _, err := os.Stat(p.val); err != nil {
			errs = append(errs, fmt.Errorf("%s: file not found: %s", p.env, p.val))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("enterprise config validation failed:\n%w", errors.Join(errs...))
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
