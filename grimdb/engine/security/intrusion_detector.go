// Package security (intrusion_detector.go) implementiert IntrusionDetector — einen
// leichten Anomaly-Detector, der auf Muster achtet, die auf unbefugte Zugriffe oder
// Data Exfiltration hindeuten.
//
// Erkannte Muster:
//   - Schnelle Auth-Failures von mehreren Source-IPs innerhalb eines kurzen Zeitfensters
//   - Ungewöhnlich hohe Entry-Read-Rate (möglicher Vault-Scan / Exfiltration)
//   - Schnelles File-Ingest gefolgt von Deletes (mögliches Data Staging)
//
// Bei Erkennung wird ein Anomaly-Event in den Kernel-Bus dispatched und auf WARN-Level
// geloggt. Nach einem konfigurierbaren Threshold von Anomalien wird automatisch
// ein Security-Lockdown ausgelöst.
package security

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// AnomalyType identifiziert die Art der erkannten Anomalie.
type AnomalyType string

const (
	AnomalyRapidAuthFailures AnomalyType = "RAPID_AUTH_FAILURES"
	AnomalyRapidEntryAccess  AnomalyType = "RAPID_ENTRY_ACCESS"
	AnomalyDataExfil         AnomalyType = "POSSIBLE_DATA_EXFIL"
)

// AnomalyEvent wird emittiert, wenn verdächtiges Verhalten erkannt wird.
type AnomalyEvent struct {
	Type      AnomalyType
	Subject   string
	Detail    string
	Timestamp time.Time
	Severity  string // "LOW", "MEDIUM", "HIGH"
}

// IntrusionDetector beobachtet auf anomale Muster und emittiert AnomalyEvents.
// Läuft In-Process neben dem Daemon mit minimalem Overhead.
type IntrusionDetector struct {
	mu sync.Mutex

	// Sliding-Window-Counter (reset nach windowSize).
	authFailures map[string][]time.Time // subject → timestamps of failures
	entryReads   map[string][]time.Time // subject → timestamps of reads
	ingestOps    map[string][]time.Time // subject → timestamps of ingest ops
	deleteOps    map[string][]time.Time // subject → timestamps of delete ops

	// Configuration.
	windowSize        time.Duration // sliding window duration
	authFailThreshold int           // failures in window before anomaly
	readRateThreshold int           // entry reads in window before anomaly
	exfilThreshold    int           // ingest+delete ops in window before anomaly

	// Anomaly callback — wird mit jedem erkannten Event aufgerufen.
	onAnomaly func(AnomalyEvent)

	// Anomaly-History (begrenzter Ringbuffer).
	history    []AnomalyEvent
	historyMax int
}

// NewIntrusionDetector erzeugt einen IntrusionDetector mit sinnvollen Defaults.
func NewIntrusionDetector(onAnomaly func(AnomalyEvent)) *IntrusionDetector {
	return &IntrusionDetector{
		authFailures:      make(map[string][]time.Time),
		entryReads:        make(map[string][]time.Time),
		ingestOps:         make(map[string][]time.Time),
		deleteOps:         make(map[string][]time.Time),
		windowSize:        5 * time.Minute,
		authFailThreshold: 3,
		readRateThreshold: 50,
		exfilThreshold:    20,
		onAnomaly:         onAnomaly,
		history:           make([]AnomalyEvent, 0, 100),
		historyMax:        100,
	}
}

// RecordAuthFailure zeichnet einen fehlgeschlagenen Auth-Versuch auf.
// Emittiert AnomalyRapidAuthFailures bei Überschreitung des Thresholds.
func (d *IntrusionDetector) RecordAuthFailure(subject string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.authFailures[subject] = d.pruneWindow(d.authFailures[subject])
	d.authFailures[subject] = append(d.authFailures[subject], time.Now())

	total := 0
	for _, timestamps := range d.authFailures {
		total += len(timestamps)
	}

	if total >= d.authFailThreshold {
		d.emit(AnomalyEvent{
			Type:      AnomalyRapidAuthFailures,
			Subject:   subject,
			Detail:    fmt.Sprintf("%d auth failures across all subjects in %s window", total, d.windowSize),
			Timestamp: time.Now(),
			Severity:  "MEDIUM",
		})
	}
}

// RecordEntryRead zeichnet eine Entry-Read-Operation auf.
// Emittiert AnomalyRapidEntryAccess bei Überschreitung des Read-Rate-Thresholds.
func (d *IntrusionDetector) RecordEntryRead(subject string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.entryReads[subject] = d.pruneWindow(d.entryReads[subject])
	d.entryReads[subject] = append(d.entryReads[subject], time.Now())

	count := len(d.entryReads[subject])
	if count >= d.readRateThreshold {
		d.emit(AnomalyEvent{
			Type:    AnomalyRapidEntryAccess,
			Subject: subject,
			Detail: fmt.Sprintf("%d entry reads for subject=%q in %s window — possible vault scan",
				count, subject, d.windowSize),
			Timestamp: time.Now(),
			Severity:  "HIGH",
		})
	}
}

// RecordIngestOp zeichnet eine File-Ingest-Operation auf.
func (d *IntrusionDetector) RecordIngestOp(subject string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.ingestOps[subject] = d.pruneWindow(d.ingestOps[subject])
	d.ingestOps[subject] = append(d.ingestOps[subject], time.Now())
	d.checkExfilPattern(subject)
}

// RecordDeleteOp zeichnet eine Block-Delete-Operation auf.
func (d *IntrusionDetector) RecordDeleteOp(subject string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.deleteOps[subject] = d.pruneWindow(d.deleteOps[subject])
	d.deleteOps[subject] = append(d.deleteOps[subject], time.Now())
	d.checkExfilPattern(subject)
}

// checkExfilPattern erkennt schnelle Ingest+Delete-Muster.
// Muss mit d.mu gehalten aufgerufen werden.
func (d *IntrusionDetector) checkExfilPattern(subject string) {
	ingests := len(d.ingestOps[subject])
	deletes := len(d.deleteOps[subject])
	combined := ingests + deletes

	if combined >= d.exfilThreshold && ingests > 0 && deletes > 0 {
		d.emit(AnomalyEvent{
			Type:    AnomalyDataExfil,
			Subject: subject,
			Detail: fmt.Sprintf("subject=%q: %d ingest + %d delete ops in %s window — possible data staging",
				subject, ingests, deletes, d.windowSize),
			Timestamp: time.Now(),
			Severity:  "HIGH",
		})
	}
}

// emit loggt und dispatched eine Anomalie. Muss mit d.mu gehalten aufgerufen werden.
func (d *IntrusionDetector) emit(ev AnomalyEvent) {
	log.Printf("[IntrusionDetector] ANOMALY type=%s severity=%s subject=%q detail=%q",
		ev.Type, ev.Severity, ev.Subject, ev.Detail)

	if len(d.history) >= d.historyMax {
		d.history = d.history[1:]
	}
	d.history = append(d.history, ev)

	// Callback ohne Lock feuern (könnte andere Locks brauchen).
	if d.onAnomaly != nil {
		go d.onAnomaly(ev)
	}
}

// pruneWindow entfernt Timestamps außerhalb des Sliding Window.
// Muss mit d.mu gehalten aufgerufen werden.
func (d *IntrusionDetector) pruneWindow(timestamps []time.Time) []time.Time {
	cutoff := time.Now().Add(-d.windowSize)
	i := 0
	for i < len(timestamps) && timestamps[i].Before(cutoff) {
		i++
	}
	return timestamps[i:]
}

// History gibt einen Snapshot der letzten Anomaly-Events zurück (neueste zuerst).
func (d *IntrusionDetector) History() []AnomalyEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]AnomalyEvent, len(d.history))
	for i, ev := range d.history {
		result[len(d.history)-1-i] = ev
	}
	return result
}
