// Package sdk — File Vault client built on top of GQLClient.
// Provides typed methods for folder/file operations using the GQL binary protocol.
package sdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/grimlocker/grimdb/engine/gql"
)

// FileEntry represents a file stored in the File Vault.
type FileEntry struct {
	ID              string `json:"id"`
	ManifestBlockID string `json:"manifest_block_id"`
	FileName        string `json:"file_name"`
	MIMEType        string `json:"mime_type"`
	TotalSize       int64  `json:"total_size"`
	FolderID        string `json:"folder_id"`
	CreatedAt       int64  `json:"created_at"`
}

// FolderItem represents a folder in the File Vault hierarchy.
type FolderItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// FolderListing contains the contents of a folder in the File Vault.
type FolderListing struct {
	FolderID   string       `json:"folder_id"`
	FolderName string       `json:"folder_name"`
	Files      []FileEntry  `json:"files"`
	Folders    []FolderItem `json:"folders"`
}

// UploadProgress reports the current status of a file upload.
type UploadProgress struct {
	BytesRead       int64  `json:"bytes_read"`
	TotalSize       int64  `json:"total_size"`
	ManifestBlockID string `json:"manifest_block_id"`
}

// sendCommand marshals a payload struct to JSON and sends it as a GQL command.
// Returns the raw JSON response bytes.
func (c *GQLClient) sendCommand(ctx context.Context, namespace string, op gql.Operation, payload interface{}) ([]byte, error) {
	var payloadJSON []byte
	var err error

	if payload != nil {
		payloadJSON, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("sdk: marshal payload: %w", err)
		}
	}

	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: op,
	}
	if payloadJSON != nil {
		query.Fields = map[string]string{"payload": string(payloadJSON)}
	}

	result, err := c.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sdk: %s failed: %s", op, result.ErrorMsg)
	}

	if len(result.Entries) > 0 && result.Entries[0].Fields != nil {
		if data, ok := result.Entries[0].Fields["payload"]; ok {
			return []byte(data), nil
		}
	}

	return []byte("{}"), nil
}

// unmarshalResponse unmarshals a JSON response into the target struct.
func unmarshalResponse(data []byte, target interface{}) error {
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("sdk: unmarshal response: %w", err)
	}
	return nil
}

// ListFolder returns the contents of a folder in the File Vault.
// Pass an empty string for folderID to list the root folder.
func (c *GQLClient) ListFolder(ctx context.Context, folderID string) (*FolderListing, error) {
	payload := map[string]string{"folder_id": folderID}
	raw, err := c.sendCommand(ctx, "default", gql.OpFileListFolder, payload)
	if err != nil {
		return nil, err
	}
	var listing FolderListing
	if err := unmarshalResponse(raw, &listing); err != nil {
		return nil, err
	}
	return &listing, nil
}

// CreateFolder creates a new folder in the File Vault.
// Pass an empty string for parentID to create at the root level.
func (c *GQLClient) CreateFolder(ctx context.Context, name, parentID string) (*FolderItem, error) {
	payload := map[string]string{"name": name, "parent_id": parentID}
	raw, err := c.sendCommand(ctx, "default", gql.OpFileCreateFolder, payload)
	if err != nil {
		return nil, err
	}
	var item FolderItem
	if err := unmarshalResponse(raw, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// RenameFolder renames a folder in the File Vault.
func (c *GQLClient) RenameFolder(ctx context.Context, folderID, name string) error {
	payload := map[string]string{"id": folderID, "name": name}
	_, err := c.sendCommand(ctx, "default", gql.OpFileRenameFolder, payload)
	return err
}

// DeleteFolder removes a folder from the File Vault.
func (c *GQLClient) DeleteFolder(ctx context.Context, folderID string) error {
	payload := map[string]string{"id": folderID}
	_, err := c.sendCommand(ctx, "default", gql.OpFileDeleteFolder, payload)
	return err
}

// MoveFile moves a file to a different folder.
func (c *GQLClient) MoveFile(ctx context.Context, manifestBlockID, folderID string) error {
	payload := map[string]string{
		"manifest_block_id": manifestBlockID,
		"folder_id":         folderID,
	}
	_, err := c.sendCommand(ctx, "default", gql.OpFileMove, payload)
	return err
}

// UploadFile uploads raw bytes as a file to the File Vault.
// The data is base64-encoded for transport through the GQL binary protocol.
func (c *GQLClient) UploadFile(ctx context.Context, namespace string, data []byte, fileName, mimeType, folderID string) (*FileEntry, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	payload := map[string]string{
		"file_name": fileName,
		"mime_type": mimeType,
		"data":      encoded,
		"folder_id": folderID,
	}
	if namespace == "" {
		namespace = "default"
	}
	raw, err := c.sendCommand(ctx, namespace, gql.OpFileIngest, payload)
	if err != nil {
		return nil, err
	}
	var entry FileEntry
	if err := unmarshalResponse(raw, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// DownloadFile retrieves file content from the File Vault by manifest block ID.
// The data is base64-decoded before being returned.
func (c *GQLClient) DownloadFile(ctx context.Context, manifestBlockID string) ([]byte, error) {
	payload := map[string]string{"manifest_block_id": manifestBlockID}
	raw, err := c.sendCommand(ctx, "default", gql.OpFileDownload, payload)
	if err != nil {
		return nil, err
	}

	var wrapped struct {
		Data string `json:"data"`
	}
	if err := unmarshalResponse(raw, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Data == "" {
		return []byte{}, nil
	}
	return base64.StdEncoding.DecodeString(wrapped.Data)
}

// UploadProgressStatus returns the current progress of a file upload.
func (c *GQLClient) UploadProgressStatus(ctx context.Context, manifestBlockID string) (*UploadProgress, error) {
	payload := map[string]string{"manifest_block_id": manifestBlockID}
	raw, err := c.sendCommand(ctx, "default", gql.OpFileUploadStatus, payload)
	if err != nil {
		return nil, err
	}
	var progress UploadProgress
	if err := unmarshalResponse(raw, &progress); err != nil {
		return nil, err
	}
	return &progress, nil
}
