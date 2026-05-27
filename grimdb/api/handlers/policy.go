package handlers

import (
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/security"
)

// PolicyManager enforces permission checks for ENTRY operations.
type PolicyManager struct {
	bus      kernel.Dispatcher
	auditLog security.AuditLog
}

// NewPolicyManager creates a PolicyManager.
func NewPolicyManager(bus kernel.Dispatcher, auditLog security.AuditLog) *PolicyManager {
	return &PolicyManager{
		bus:      bus,
		auditLog: auditLog,
	}
}

// CheckWrite returns true if the subject is allowed to write (create/update/delete).
// Default policy: "daemon" subject always allowed; empty subject always denied.
func (p *PolicyManager) CheckWrite(subjectID string) bool {
	// Default: daemon subject (internal operations) always allowed
	if subjectID == "daemon" {
		return true
	}
	// Empty subject: always deny (not authenticated)
	if subjectID == "" {
		return false
	}
	// Future: more sophisticated permission model can be added here
	// For now: allow authenticated subjects
	return true
}

// CheckRead returns true if the subject is allowed to read.
// Default policy: all authenticated subjects can read.
func (p *PolicyManager) CheckRead(subjectID string) bool {
	// Deny empty (unauthenticated)
	return subjectID != ""
}

// OnUnauthorized logs an unauthorized access attempt and dispatches an audit event.
func (p *PolicyManager) OnUnauthorized(subjectID, operation string) {
	// Log to audit ring buffer
	p.auditLog.Append(security.SecurityEvent{
		Level:     security.LevelCritical,
		Module:    "policy",
		Message:   "UNAUTHORIZED_ACCESS: " + operation,
		SubjectID: subjectID,
	})

	// Dispatch audit event to the bus (for external listeners)
	ev := kernel.NewEvent("policy", kernel.EvSecAudit, []byte(operation))
	_ = p.bus.Dispatch(ev)
}
