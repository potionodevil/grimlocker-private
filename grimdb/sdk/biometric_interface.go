package sdk

// BiometricToken is the only biometric artefact that crosses a module boundary.
// Raw sensor data (images, iris scans, waveforms) MUST be processed entirely
// inside the BiometricProvider implementation and never appear here.
//
// The kernel verifies only the cryptographic signature; it never inspects the
// underlying biometric material.
type BiometricToken struct {
	// SensorID is the hardware identifier of the sensor that produced this token.
	SensorID string `json:"sensor_id"`

	// Algorithm names the signing scheme (e.g. "Ed25519", "ECDSA-P256-SHA256").
	Algorithm string `json:"algorithm"`

	// Timestamp is Unix nanoseconds. Used to enforce an anti-replay window.
	Timestamp int64 `json:"timestamp"`

	// Nonce is 16 random bytes included in the signed payload to prevent replay.
	Nonce []byte `json:"nonce"`

	// Signature is the cryptographic signature over:
	//   SHA-256(SensorID || Timestamp_bytes || Nonce)
	// produced by the sensor's private key.
	Signature []byte `json:"signature"`
}

// BiometricProvider is the interface every biometric plugin must implement.
// The contract guarantees that raw biometric data is contained within the
// implementation and only signed tokens are exposed to the rest of the system.
type BiometricProvider interface {
	// SensorID returns the stable hardware identifier for this sensor.
	SensorID() string

	// Algorithm returns the signing algorithm name (e.g. "Ed25519").
	Algorithm() string

	// Authenticate performs a live biometric capture, produces a signed token,
	// and returns it. The underlying sensor data is discarded before returning.
	Authenticate() (BiometricToken, error)

	// Verify checks the token's signature against the sensor's public key and
	// validates the anti-replay constraints (timestamp window, nonce uniqueness).
	// MUST execute in constant time with respect to signature content.
	Verify(token BiometricToken) error
}

// BiometricPlugin extends sdk.Plugin for hardware sensor modules.
// Register via sdk.Registry.Register — the SDK dispatcher enforces that
// BIOMETRIC.* events are accessible only to biometric plugins.
type BiometricPlugin interface {
	Plugin

	// BiometricProvider returns the hardware sensor abstraction.
	BiometricProvider() BiometricProvider
}

// AntiReplayWindow is the maximum age of a valid BiometricToken.
// Tokens older than this are rejected by Verify implementations.
const AntiReplayWindow = 30e9 // 30 seconds in nanoseconds
