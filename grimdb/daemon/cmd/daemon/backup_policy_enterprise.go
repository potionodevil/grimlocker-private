//go:build enterprise

package main

import (
	"fmt"

	backupmod "github.com/grimlocker/grimdb/daemon/internal/modules/backup"
)

// backupExportPolicy gibt eine Policy zurück, die Backup-Operationen auf Admins beschränkt.
// origin enthält die Rolle des authentifizierten Users ("admin" | "user").
func backupExportPolicy() backupmod.ExportPolicyFn {
	return func(origin string) error {
		if origin != "admin" {
			return fmt.Errorf("backup export/import ist Enterprise-Admins vorbehalten")
		}
		return nil
	}
}
