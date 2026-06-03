//go:build enterprise

package remote

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
)

// s3Client is the minimal S3 API surface used by RemoteVault.
type s3Client interface {
	putObject(ctx context.Context, bucket, key string, body io.Reader, size int64) error
	getObject(ctx context.Context, bucket, key string) ([]byte, error)
	deleteObject(ctx context.Context, bucket, key string) error
	headObject(ctx context.Context, bucket, key string) (bool, error)
}

// RemoteVault implements storage.BlockStore backed by an S3-compatible object store.
// Blocks are AES-256-GCM encrypted with the MVK before upload.
type RemoteVault struct {
	mu      sync.RWMutex
	s3      s3Client
	bucket  string
	cache   *blockCache
	index   *remoteIndex
	getMVK  func() []byte
	adapter kernel.Module
}

// RemoteVaultConfig holds S3 connection parameters.
type RemoteVaultConfig struct {
	Endpoint   string // empty = AWS default (https://<bucket>.s3.<region>.amazonaws.com)
	Region     string
	Bucket     string
	AccessKey  string
	SecretKey  string
	CacheBytes int // 0 = 100 MB default
}

// NewRemoteVault creates a RemoteVault connected to the given S3-compatible endpoint.
func NewRemoteVault(cfg RemoteVaultConfig) (*RemoteVault, error) {
	s3c := &httpS3Client{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
	return &RemoteVault{
		s3:     s3c,
		bucket: cfg.Bucket,
		cache:  newBlockCache(cfg.CacheBytes),
		index:  newRemoteIndex(),
	}, nil
}

func (v *RemoteVault) SetMVKFunc(fn func() []byte) {
	v.mu.Lock()
	v.getMVK = fn
	v.mu.Unlock()
}

func (v *RemoteVault) LoadIndex() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	mvk := v.getMVK()
	if mvk == nil {
		return fmt.Errorf("MVK not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return v.index.load(ctx, v.s3, v.bucket, mvk)
}

func (v *RemoteVault) WriteBlock(b storage.Block) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	mvk := v.getMVK()
	if mvk == nil {
		return fmt.Errorf("vault locked")
	}
	plaintext, err := json.Marshal(b)
	if err != nil {
		return fmt.Errorf("marshal block: %w", err)
	}
	ct, err := aesgcmEncrypt(mvk, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt block: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := v.s3.putObject(ctx, v.bucket, blockObjectKey(b.ID), bytes.NewReader(ct), int64(len(ct))); err != nil {
		return fmt.Errorf("upload block %s: %w", b.ID, err)
	}
	v.index.set(storage.BlockMeta{ID: b.ID, Category: b.Category, Size: int64(len(b.Data))})
	v.cache.put(b.ID, ct)
	return v.index.save(ctx, v.s3, v.bucket, mvk)
}

func (v *RemoteVault) ReadBlock(id string) (storage.Block, error) {
	v.mu.RLock()
	mvk := v.getMVK()
	v.mu.RUnlock()
	if mvk == nil {
		return storage.Block{}, fmt.Errorf("vault locked")
	}
	if ct, ok := v.cache.get(id); ok {
		return decryptBlock(mvk, ct)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ct, err := v.s3.getObject(ctx, v.bucket, blockObjectKey(id))
	if err != nil {
		return storage.Block{}, fmt.Errorf("download block %s: %w", id, err)
	}
	b, err := decryptBlock(mvk, ct)
	if err != nil {
		return storage.Block{}, err
	}
	v.cache.put(id, ct)
	return b, nil
}

func (v *RemoteVault) DeleteBlock(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	mvk := v.getMVK()
	if mvk == nil {
		return fmt.Errorf("vault locked")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := v.s3.deleteObject(ctx, v.bucket, blockObjectKey(id)); err != nil {
		return fmt.Errorf("delete block %s: %w", id, err)
	}
	v.index.delete(id)
	v.cache.evict(id)
	return v.index.save(ctx, v.s3, v.bucket, mvk)
}

func (v *RemoteVault) ListBlocks() ([]storage.BlockMeta, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.index.list(), nil
}

func (v *RemoteVault) QueryBlocks(cat storage.Category) ([]storage.BlockMeta, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.index.query(cat), nil
}

func (v *RemoteVault) Flush() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	mvk := v.getMVK()
	if mvk == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return v.index.save(ctx, v.s3, v.bucket, mvk)
}

func (v *RemoteVault) Close() error { return v.Flush() }

func (v *RemoteVault) KernelModule() kernel.Module    { return v.adapter }
func (v *RemoteVault) SetKernelModule(m kernel.Module) { v.adapter = m }

func (v *RemoteVault) TestConnectivity() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := v.s3.headObject(ctx, v.bucket, "_health")
	if err != nil {
		log.Printf("[remote-vault] connectivity check: %v (expected for empty vault)", err)
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func blockObjectKey(id string) string { return "blocks/" + id + "/data.enc" }

func decryptBlock(mvk, ct []byte) (storage.Block, error) {
	plaintext, err := aesgcmDecrypt(mvk, ct)
	if err != nil {
		return storage.Block{}, fmt.Errorf("decrypt block: %w", err)
	}
	var b storage.Block
	if err := json.Unmarshal(plaintext, &b); err != nil {
		return storage.Block{}, fmt.Errorf("unmarshal block: %w", err)
	}
	return b, nil
}

// ── Minimal AWS SigV4 S3 client (stdlib only) ─────────────────────────────────

type httpS3Client struct {
	cfg    RemoteVaultConfig
	client *http.Client
}

func (c *httpS3Client) objectURL(bucket, key string) string {
	if c.cfg.Endpoint != "" {
		return fmt.Sprintf("%s/%s/%s", c.cfg.Endpoint, bucket, key)
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, c.cfg.Region, key)
}

func (c *httpS3Client) putObject(ctx context.Context, bucket, key string, body io.Reader, size int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.objectURL(bucket, key), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	c.sign(req, data)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("S3 PUT %s: %d: %s", key, resp.StatusCode, rb)
	}
	return nil
}

func (c *httpS3Client) getObject(ctx context.Context, bucket, key string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.objectURL(bucket, key), nil)
	if err != nil {
		return nil, err
	}
	c.sign(req, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found: %s", key)
	}
	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("S3 GET %s: %d: %s", key, resp.StatusCode, rb)
	}
	return io.ReadAll(resp.Body)
}

func (c *httpS3Client) deleteObject(ctx context.Context, bucket, key string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.objectURL(bucket, key), nil)
	if err != nil {
		return err
	}
	c.sign(req, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("S3 DELETE %s: %d", key, resp.StatusCode)
	}
	return nil
}

func (c *httpS3Client) headObject(ctx context.Context, bucket, key string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.objectURL(bucket, key), nil)
	if err != nil {
		return false, err
	}
	c.sign(req, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// sign adds AWS Signature Version 4 headers to the request.
func (c *httpS3Client) sign(req *http.Request, body []byte) {
	if body == nil {
		body = []byte{}
	}
	now := time.Now().UTC()
	date := now.Format("20060102")
	datetime := now.Format("20060102T150405Z")
	region := c.cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	payloadHash := fmt.Sprintf("%x", sha256.Sum256(body))
	req.Header.Set("x-amz-date", datetime)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("Host", req.URL.Host)

	canonicalHeaders := "host:" + req.URL.Host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + datetime + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalReq := req.Method + "\n" +
		req.URL.EscapedPath() + "\n" +
		"\n" + canonicalHeaders + "\n" +
		signedHeaders + "\n" + payloadHash

	credScope := date + "/" + region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + datetime + "\n" + credScope + "\n" +
		fmt.Sprintf("%x", sha256.Sum256([]byte(canonicalReq)))

	signingKey := sigv4Key(c.cfg.SecretKey, date, region, "s3")
	signature := fmt.Sprintf("%x", hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.cfg.AccessKey, credScope, signedHeaders, signature))
}

func sigv4Key(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
