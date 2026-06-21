package security

// constantTimeEqual gibt true zurück, wenn a und b die gleiche Länge UND den gleichen Inhalt haben.
// Die Laufzeit hängt nur von len(a) und len(b) ab, nicht vom Inhalt — schützt vor Timing-Angriffen.
func constantTimeEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
