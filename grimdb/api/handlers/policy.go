package handlers

import (
	"log"

	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/security"
)

// PolicyManager enforces permission checks for ENTRY operations.
// All denied decisions are logged with subject_id and operation for diagnostics.
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
// Decision is logged at DEBUG level for diagnostics.
func (p *PolicyManager) CheckWrite(subjectID string) bool {
	// Default: daemon subject (internal operations) always allowed
	if subjectID == "daemon" {
		log.Printf("[Policy:CheckWrite] ALLOW subject=%q (daemon)", subjectID)
		return true
	}
	// Empty subject: always deny (not authenticated — subject_id was not sent)
	if subjectID == "" {
		log.Printf("[Policy:CheckWrite] DENY subject=%q (empty — unauthenticated)", subjectID)
		return false
	}
	// All authenticated subjects (non-empty, non-daemon) are allowed to write.
	// Future: replace with role-based check here.
	log.Printf("[Policy:CheckWrite] ALLOW subject=%q", subjectID)
	return true
}

// CheckRead returns true if the subject is allowed to read.
// Default policy: all authenticated subjects can read (non-empty subjectID).
func (p *PolicyManager) CheckRead(subjectID string) bool {
	allowed := subjectID != ""
	if !allowed {
		log.Printf("[Policy:CheckRead] DENY subject=%q (empty — unauthenticated)", subjectID)
	}
	return allowed
}

// OnUnauthorized logs an unauthorized access attempt to stderr, the audit ring
// buffer, and the security event bus. The log line includes all fields needed
// for diagnosis: subject_id, operation, and a stacktrace marker.
func (p *PolicyManager) OnUnauthorized(subjectID, operation string) {
	log.Printf("[Policy:UNAUTHORIZED] subject_id=%q operation=%q — access denied", subjectID, operation)

	// Log to audit ring buffer (hash-chained, tamper-evident)
	p.auditLog.Append(security.SecurityEvent{
		Level:     security.LevelCritical,
		Module:    "policy",
		Message:   "UNAUTHORIZED_ACCESS: " + operation,
		SubjectID: subjectID,
	})

	// Dispatch SECURITY.AUDIT event to the bus (for external listeners / watchdog)
	ev := kernel.NewEvent("policy", kernel.EvSecAudit, []byte(operation))
	_ = p.bus.Dispatch(ev)
}
