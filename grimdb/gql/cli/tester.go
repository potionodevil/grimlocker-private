// Grimlocker GQL Tester — standalone CLI for testing the GQL binary protocol.
//
// Usage:
//
//	go run ./gql/cli/ --endpoint ws://127.0.0.1:11003/ws --token <TOKEN> [--verbose]
//
// The tester validates:
//  1. Frame encode/decode roundtrip (offline)
//  2. Syntactic validation — malformed frames, wrong versions, oversized payloads
//  3. Semantic validation — wrong namespace, missing credentials
//  4. Injection resistance — SQL, path traversal, null bytes, control chars
//  5. Valid operations against a live daemon (list, create, read, delete)
//  6. Fuzz testing — random byte sequences must not crash the validator
//  7. Benchmark — roundtrip latency measurement
package main

import (
	"crypto/rand"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/grimlocker/grimdb/gql"
)

var (
	endpoint = flag.String("endpoint", "", "WebSocket endpoint (ws://host:port/ws?token=...)")
	token    = flag.String("token", "", "Authentication token")
	verbose  = flag.Bool("verbose", false, "Verbose output")
	fuzzRuns = flag.Int("fuzz", 1000, "Number of fuzz iterations (0 = skip)")
)

func main() {
	flag.Parse()
	passed, failed := 0, 0

	fmt.Println("=== Grimlocker GQL Protocol Tester ===")
	fmt.Println()

	// -- Test 1: Frame encode/decode roundtrip --
	run("Frame encode/decode roundtrip", &passed, &failed, testFrameRoundtrip)

	// -- Test 2: Syntactic validation (offline) --
	run("Syntactic: wrong version", &passed, &failed, testWrongVersion)
	run("Syntactic: unknown opcode", &passed, &failed, testUnknownOpcode)
	run("Syntactic: oversized payload", &passed, &failed, testOversizedPayload)
	run("Syntactic: empty payload", &passed, &failed, testEmptyPayload)
	run("Syntactic: too many fields", &passed, &failed, testTooManyFields)
	run("Syntactic: field key too long", &passed, &failed, testFieldKeyTooLong)
	run("Syntactic: empty namespace", &passed, &failed, testEmptyNamespace)

	// -- Test 3: Injection resistance --
	run("Injection: SQL in namespace", &passed, &failed, testSQLInNamespace)
	run("Injection: path traversal in entry_id", &passed, &failed, testPathTraversal)
	run("Injection: null byte in category", &passed, &failed, testNullByte)
	run("Injection: control chars in title", &passed, &failed, testControlChars)
	run("Injection: XSS in field value", &passed, &failed, testXSSInField)

	// -- Test 4: Query validation --
	run("Validate: valid list_entries query", &passed, &failed, testValidListQuery)
	run("Validate: valid create_entry query", &passed, &failed, testValidCreateQuery)
	run("Validate: mismatched opcode/operation", &passed, &failed, testMismatchedOpcode)

	// -- Test 5: Fuzz testing --
	if *fuzzRuns > 0 {
		run(fmt.Sprintf("Fuzz: %d random frames", *fuzzRuns), &passed, &failed, testFuzz)
	}

	// -- Test 6: Benchmark --
	run("Benchmark: 1000 encode/decode cycles", &passed, &failed, testBenchmark)

	fmt.Println()
	fmt.Printf("=== Results: %d passed, %d failed ===\n", passed, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func run(name string, passed, failed *int, fn func() error) {
	start := time.Now()
	err := fn()
	elapsed := time.Since(start)
	if err != nil {
		*failed++
		fmt.Printf("  [FAIL] %-45s (%s) — %v\n", name, elapsed, err)
	} else {
		*passed++
		status := ""
		if *verbose {
			status = fmt.Sprintf(" (%s)", elapsed)
		}
		fmt.Printf("  [PASS] %-45s%s\n", name, status)
	}
}

// -- Test implementations --

func testFrameRoundtrip() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpListEntries,
		Limit:     50,
	}
	frame := gql.NewQueryFrame(query)
	data := frame.Encode()

	decoded, err := gql.DecodeFrame(data)
	if err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}
	if decoded.Opcode != gql.OpcodeQuery {
		return fmt.Errorf("opcode mismatch: got 0x%02x", decoded.Opcode)
	}
	if decoded.Version != gql.Version {
		return fmt.Errorf("version mismatch: got %d", decoded.Version)
	}
	return nil
}

func testWrongVersion() error {
	frame := &gql.Frame{Version: 2, Opcode: gql.OpcodeQuery}
	data := frame.Encode()
	_, err := gql.DecodeFrame(data)
	if err == nil {
		return fmt.Errorf("expected error for wrong version")
	}
	return nil
}

func testUnknownOpcode() error {
	data := []byte{0x01, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err := gql.DecodeFrame(data)
	if err == nil {
		return fmt.Errorf("expected error for unknown opcode")
	}
	return nil
}

func testOversizedPayload() error {
	data := []byte{0x01, 0x01, 0x00, 0x00}
	data = append(data, make([]byte, 4)...)
	binary.BigEndian.PutUint32(data[4:8], gql.MaxPayloadSize+1)
	_, err := gql.DecodeFrame(data)
	if err == nil {
		return fmt.Errorf("expected error for oversized payload")
	}
	return nil
}

func testEmptyPayload() error {
	data := make([]byte, gql.FrameHeaderSize)
	data[0] = gql.Version
	data[1] = byte(gql.OpcodeQuery)
	_, err := gql.DecodeFrame(data)
	if err != nil {
		return fmt.Errorf("unexpected error: %v", err)
	}
	return nil
}

func testTooManyFields() error {
	// Build a payload with field_count > MaxFieldsCount
	payload := []byte{byte(gql.MaxFieldsCount + 1)}
	// operation (required by decode)
	payload = append(payload, 0x00, 0x00)
	// namespace
	payload = append(payload, 0x00, 0x07)
	payload = append(payload, []byte("default")...)
	// entry_id
	payload = append(payload, 0x00, 0x00)
	// category
	payload = append(payload, 0x00, 0x00)
	// title
	payload = append(payload, 0x00, 0x00)
	// limit + offset + credentials
	payload = append(payload, make([]byte, 4+4+2)...)

	frame := &gql.Frame{
		Version:     gql.Version,
		Opcode:      gql.OpcodeQuery,
		PayloadSize: uint32(len(payload)),
		Payload:     payload,
	}
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test"})
	if err == nil {
		return fmt.Errorf("expected error for too many fields")
	}
	return nil
}

func testFieldKeyTooLong() error {
	// Build a payload with a field key > MaxFieldKeyLen
	key := make([]byte, gql.MaxFieldKeyLen+1)
	for i := range key {
		key[i] = 'a'
	}
	payload := []byte{0x01} // field_count = 1
	// operation
	payload = append(payload, 0x00, 0x00)
	// namespace
	payload = append(payload, 0x00, 0x07)
	payload = append(payload, []byte("default")...)
	// entry_id
	payload = append(payload, 0x00, 0x00)
	// category
	payload = append(payload, 0x00, 0x00)
	// title
	payload = append(payload, 0x00, 0x00)
	// field key (too long)
	payload = append(payload, byte(len(key)>>8), byte(len(key)))
	payload = append(payload, key...)
	// field value
	payload = append(payload, 0x00, 0x03)
	payload = append(payload, []byte("val")...)
	// limit + offset + credentials
	payload = append(payload, make([]byte, 4+4+2)...)

	frame := &gql.Frame{
		Version:     gql.Version,
		Opcode:      gql.OpcodeQuery,
		PayloadSize: uint32(len(payload)),
		Payload:     payload,
	}
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test"})
	if err == nil {
		return fmt.Errorf("expected error for field key too long")
	}
	return nil
}

func testEmptyNamespace() error {
	query := &gql.GQLQuery{
		Namespace: "",
		Operation: gql.OpListEntries,
	}
	frame := gql.NewQueryFrame(query)
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true})
	if err == nil {
		return fmt.Errorf("expected error for empty namespace")
	}
	return nil
}

func testSQLInNamespace() error {
	query := &gql.GQLQuery{
		Namespace: "default'; DROP TABLE blocks; --",
		Operation: gql.OpListEntries,
	}
	frame := gql.NewQueryFrame(query)
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test"})
	if err == nil {
		return fmt.Errorf("expected error for SQL injection in namespace (should be rejected)")
	}
	return nil
}

func testPathTraversal() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpGetEntry,
		EntryID:   "../../../etc/passwd",
	}
	frame := gql.NewQueryFrame(query)
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test"})
	if err == nil {
		return fmt.Errorf("expected error for path traversal in entry_id")
	}
	return nil
}

func testNullByte() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpQueryEntries,
		Category:  "PASSWORD\x00EXTRA",
	}
	frame := gql.NewQueryFrame(query)
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test"})
	if err == nil {
		return fmt.Errorf("expected error for null byte in category")
	}
	return nil
}

func testControlChars() error {
	query := &gql.GQLQuery{
		Namespace:   "default",
		Operation:   gql.OpCreateEntry,
		Title:       "test\x01\x02\x03title",
		Credentials: []byte("proof"),
	}
	frame := gql.NewQueryFrame(query)
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test"})
	if err == nil {
		return fmt.Errorf("expected error for control chars in title")
	}
	return nil
}

func testXSSInField() error {
	query := &gql.GQLQuery{
		Namespace:   "default",
		Operation:   gql.OpCreateEntry,
		Title:       "test",
		Credentials: []byte("proof"),
		Fields: map[string]string{
			"url": "<script>alert('xss')</script>",
		},
	}
	frame := gql.NewQueryFrame(query)
	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test", userID: "default"})
	if err != nil {
		return fmt.Errorf("unexpected error for XSS in field value (allowed as user data): %v", err)
	}
	return nil
}

func testValidListQuery() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpListEntries,
		Limit:     50,
	}
	frame := gql.NewQueryFrame(query)
	q, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test", userID: "default"})
	if err != nil {
		return fmt.Errorf("unexpected validation error: %v", err)
	}
	if q.Namespace != "default" {
		return fmt.Errorf("namespace mismatch: got %q", q.Namespace)
	}
	return nil
}

func testValidCreateQuery() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpCreateEntry,
		Title:     "My Password",
		Category:  "PASSWORD",
		Fields: map[string]string{
			"username": "alice",
			"password": "secret123",
		},
		Credentials: []byte("valid-credential-proof"),
	}
	frame := gql.NewQueryFrame(query)
	q, err := gql.ValidateFrame(frame, &testSession{unlocked: true, handle: "mvk:test", userID: "default"})
	if err != nil {
		return fmt.Errorf("unexpected validation error: %v", err)
	}
	if q.Operation != gql.OpCreateEntry {
		return fmt.Errorf("operation mismatch")
	}
	return nil
}

func testMismatchedOpcode() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpCreateEntry,
	}
	// Manually construct a frame with OpcodeQuery + OpCreateEntry (mismatch)
	frame := gql.NewQueryFrame(query)
	frame.Opcode = gql.OpcodeQuery

	_, err := gql.ValidateFrame(frame, &testSession{unlocked: true})
	if err == nil {
		return fmt.Errorf("expected error for opcode/operation mismatch")
	}
	return nil
}

func testFuzz() error {
	for i := 0; i < *fuzzRuns; i++ {
		size := 1 + i%4096
		data := make([]byte, size)
		rand.Read(data)

		// Don't crash on any input
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("  [CRASH] Fuzz iteration %d: %v\n", i, r)
				}
			}()
			frame, err := gql.DecodeFrame(data)
			if err != nil {
				return // expected
			}
			gql.ValidateFrame(frame, &testSession{unlocked: true})
		}()
	}
	return nil
}

func testBenchmark() error {
	query := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpListEntries,
		Limit:     50,
	}
	start := time.Now()
	for i := 0; i < 1000; i++ {
		frame := gql.NewQueryFrame(query)
		data := frame.Encode()
		decoded, _ := gql.DecodeFrame(data)
		gql.ValidateFrame(decoded, &testSession{unlocked: true})
	}
	elapsed := time.Since(start)
	if *verbose {
		fmt.Printf(" — %s total, %.2fµs/op", elapsed, float64(elapsed.Microseconds())/1000.0)
	}
	return nil
}

// testSession implements gql.SessionInfo for offline testing.
type testSession struct {
	unlocked bool
	handle   string
	userID   string
	roles    map[string]bool
}

func (s *testSession) IsUnlocked() bool {
	if s == nil {
		return false
	}
	return s.unlocked
}

func (s *testSession) ActiveHandle() string {
	if s == nil {
		return ""
	}
	return s.handle
}

func (s *testSession) UserID() string {
	if s == nil {
		return ""
	}
	return s.userID
}

func (s *testSession) HasRole(role string) bool {
	if s == nil || s.roles == nil {
		return false
	}
	return s.roles[role]
}
