//go:build !enterprise

package main

import backupmod "github.com/grimlocker/grimdb/daemon/internal/modules/backup"

// backupExportPolicy gibt nil zurück — Single-User darf immer exportieren/importieren.
func backupExportPolicy() backupmod.ExportPolicyFn { return nil }
