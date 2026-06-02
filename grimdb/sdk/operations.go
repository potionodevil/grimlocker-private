// Package sdk — typed high-level helpers built on top of GQLClient.
// These wrap the generic Create/List/Get methods with domain-specific structs.
package sdk

import (
	"context"
	"fmt"

	"github.com/grimlocker/grimdb/gql"
)

// PasswordEntry represents a stored password credential.
type PasswordEntry struct {
	ID       string
	Title    string
	Username string
	Password string
	URL      string
	Notes    string
}

// SSHKeyEntry represents a stored SSH key pair.
type SSHKeyEntry struct {
	ID        string
	Title     string
	PublicKey string
	Comment   string
	Algorithm string
}

// CertificateEntry represents a stored TLS certificate.
type CertificateEntry struct {
	ID          string
	Title       string
	Domain      string
	Certificate string
	PrivateKey  string
}

// CreatePassword stores a new password entry and returns its assigned ID.
func (c *GQLClient) CreatePassword(ctx context.Context, namespace string, p *PasswordEntry) (string, error) {
	entry, err := c.CreateEntry(ctx, namespace, p.Title, "PASSWORD", map[string]string{
		"username": p.Username,
		"password": p.Password,
		"url":      p.URL,
		"notes":    p.Notes,
	})
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

// ListPasswords returns all password entries in the given namespace.
func (c *GQLClient) ListPasswords(ctx context.Context, namespace string) ([]PasswordEntry, error) {
	raw, err := c.QueryEntries(ctx, namespace, "PASSWORD")
	if err != nil {
		return nil, err
	}
	out := make([]PasswordEntry, 0, len(raw))
	for _, e := range raw {
		out = append(out, passwordFromEntry(e))
	}
	return out, nil
}

// GetPassword retrieves a single password entry by ID.
func (c *GQLClient) GetPassword(ctx context.Context, namespace, id string) (*PasswordEntry, error) {
	e, err := c.GetEntry(ctx, namespace, id)
	if err != nil {
		return nil, err
	}
	if e.Category != "PASSWORD" {
		return nil, fmt.Errorf("sdk: entry %s is category %q, not PASSWORD", id, e.Category)
	}
	p := passwordFromEntry(*e)
	return &p, nil
}

// CreateSSHKey stores a new SSH key entry and returns its assigned ID.
func (c *GQLClient) CreateSSHKey(ctx context.Context, namespace string, k *SSHKeyEntry) (string, error) {
	entry, err := c.CreateEntry(ctx, namespace, k.Title, "SSH_KEY", map[string]string{
		"public_key": k.PublicKey,
		"comment":    k.Comment,
		"algorithm":  k.Algorithm,
	})
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

// ListSSHKeys returns all SSH key entries in the given namespace.
func (c *GQLClient) ListSSHKeys(ctx context.Context, namespace string) ([]SSHKeyEntry, error) {
	raw, err := c.QueryEntries(ctx, namespace, "SSH_KEY")
	if err != nil {
		return nil, err
	}
	out := make([]SSHKeyEntry, 0, len(raw))
	for _, e := range raw {
		out = append(out, sshKeyFromEntry(e))
	}
	return out, nil
}

// GetSSHKey retrieves a single SSH key entry by ID.
func (c *GQLClient) GetSSHKey(ctx context.Context, namespace, id string) (*SSHKeyEntry, error) {
	e, err := c.GetEntry(ctx, namespace, id)
	if err != nil {
		return nil, err
	}
	if e.Category != "SSH_KEY" {
		return nil, fmt.Errorf("sdk: entry %s is category %q, not SSH_KEY", id, e.Category)
	}
	k := sshKeyFromEntry(*e)
	return &k, nil
}

// CreateCertificate stores a new certificate entry and returns its assigned ID.
func (c *GQLClient) CreateCertificate(ctx context.Context, namespace string, cert *CertificateEntry) (string, error) {
	entry, err := c.CreateEntry(ctx, namespace, cert.Title, "CERTIFICATE", map[string]string{
		"domain":       cert.Domain,
		"certificate":  cert.Certificate,
		"private_key":  cert.PrivateKey,
	})
	if err != nil {
		return "", err
	}
	return entry.ID, nil
}

// ListCertificates returns all certificate entries in the given namespace.
func (c *GQLClient) ListCertificates(ctx context.Context, namespace string) ([]CertificateEntry, error) {
	raw, err := c.QueryEntries(ctx, namespace, "CERTIFICATE")
	if err != nil {
		return nil, err
	}
	out := make([]CertificateEntry, 0, len(raw))
	for _, e := range raw {
		out = append(out, certFromEntry(e))
	}
	return out, nil
}

// GetCertificate retrieves a single certificate entry by ID.
func (c *GQLClient) GetCertificate(ctx context.Context, namespace, id string) (*CertificateEntry, error) {
	e, err := c.GetEntry(ctx, namespace, id)
	if err != nil {
		return nil, err
	}
	if e.Category != "CERTIFICATE" {
		return nil, fmt.Errorf("sdk: entry %s is category %q, not CERTIFICATE", id, e.Category)
	}
	cert := certFromEntry(*e)
	return &cert, nil
}

// SearchEntries performs a text search across entries, optionally filtered by category.
func (c *GQLClient) SearchEntries(ctx context.Context, namespace, query, category string) ([]gql.GQLEntry, error) {
	q := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpSearchEntries,
		Category:  category,
		Fields: map[string]string{
			"search": query,
		},
	}
	result, err := c.Execute(ctx, q)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sdk: search failed: %s", result.ErrorMsg)
	}
	return result.Entries, nil
}

func passwordFromEntry(e gql.GQLEntry) PasswordEntry {
	return PasswordEntry{
		ID:       e.ID,
		Title:    e.Title,
		Username: e.Fields["username"],
		Password: e.Fields["password"],
		URL:      e.Fields["url"],
		Notes:    e.Fields["notes"],
	}
}

func sshKeyFromEntry(e gql.GQLEntry) SSHKeyEntry {
	return SSHKeyEntry{
		ID:        e.ID,
		Title:     e.Title,
		PublicKey: e.Fields["public_key"],
		Comment:   e.Fields["comment"],
		Algorithm: e.Fields["algorithm"],
	}
}

func certFromEntry(e gql.GQLEntry) CertificateEntry {
	return CertificateEntry{
		ID:          e.ID,
		Title:       e.Title,
		Domain:      e.Fields["domain"],
		Certificate: e.Fields["certificate"],
		PrivateKey:  e.Fields["private_key"],
	}
}
