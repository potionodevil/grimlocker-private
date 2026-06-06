// Package errors bietet das einheitliche getypte Error-System für Grimlocker Omega+.
//
// Jedes Module gibt *GrimlockError zurück statt plain Go-Fehler.
// Damit bekommt jeder Fehler einen numerischen Code, strukturierten Kontext
// (Block-ID, Operation-Name, Key-Value-Details), optional einen Stacktrace
// am Error-Entstehungsort und ein HTTP-Status-Mapping für die REST/WebSocket-API.
//
// Error-Code-Bereiche:
//
//	1000–1999  Vault / Authentication
//	2000–2999  Storage / GrimDB
//	3000–3999  Cryptography
//	4000–4999  Security / Lockdown
//	5000–5999  Kernel / Bus
//	6000–6999  API / Protocol
//
// Kurz-Usage:
//
//	return gerrors.NewStorageIOError("read_block", blockID, err)
//
//	return gerrors.NewAuthInvalidError("jwt_verification", err).
//	    WithModule("oidc-auth").
//	    WithDetails("subject", claims.Subject)
//
//	wrapped := gerrors.Wrap(gerrors.ErrCodeStorageIO, "blockstore failed", err)
//
// Siehe docs/ERROR_CODES.md für eine vollständige Code-Referenz mit Recovery-Schritten.
// Siehe docs/API_REFERENCE.md für die komplette GrimlockError-Struct/Methoden-Doku.
package errors

import (
	"fmt"
	"runtime"
)

const maxStackFrames = 20

// StackFrame ist ein einzelner Call-Stack-Frame.
type StackFrame struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

func (f StackFrame) String() string {
	return fmt.Sprintf("%s:%d in %s", f.File, f.Line, f.Function)
}

// CaptureStacktrace erfasst bis zu maxStackFrames des Call-Stacks,
// überspringt dabei `skip` zusätzliche Frames oberhalb des Callers.
// skip=0 → erster Frame ist der direkte Caller von CaptureStacktrace.
// skip=1 → erster Frame ist der Caller des Callers (nutze das von Error-Constructors).
func CaptureStacktrace(skip int) []StackFrame {
	pcs := make([]uintptr, maxStackFrames)
	n := runtime.Callers(skip+2, pcs)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	result := make([]StackFrame, 0, n)
	for {
		frame, more := frames.Next()
		result = append(result, StackFrame{
			File:     frame.File,
			Line:     frame.Line,
			Function: frame.Function,
		})
		if !more {
			break
		}
	}
	return result
}
