package gql

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// GQLQuery ist die deserialisierte Form eines GQL-Query- oder Mutate-Requests.
// Das ist die kanonische interne Repräsentation zwischen Frame-Decoder, Validator und Dispatcher.
type GQLQuery struct {
	Namespace   string            // Workspace oder User-ID (required)
	Operation   Operation         // Die auszuführende GQL-Operation
	EntryID     string            // Target-Entry-ID (für get/update/delete)
	Category    string            // Filter-Category (PASSWORD, SSH_KEY, etc.)
	Title       string            // Entry-Title (für create/update)
	Fields      map[string]string // Key-Value-Felder für den Entry
	Credentials []byte            // SKE-encrypted MVK-Handle-Proof (für write-ops)
	Limit       uint32            // Max-Ergebnisse, 0 = default (50)
	Offset      uint32            // Pagination-Offset
}

// GQLEntry ist ein einzelner Entry in einem GQL-Resultset.
type GQLEntry struct {
	ID        string            `json:"id"`
	Category  string            `json:"category"`
	Type      string            `json:"type,omitempty"`
	Title     string            `json:"title,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
}

// GQLResult ist die Server-Response für eine GQL-Query oder -Mutation.
type GQLResult struct {
	RequestID  string     `json:"request_id,omitempty"`
	Success    bool       `json:"success"`
	Entries    []GQLEntry `json:"entries,omitempty"`
	TotalCount uint32     `json:"total_count,omitempty"`
	ErrorCode  int32      `json:"error_code,omitempty"`
	ErrorMsg   string     `json:"error_msg,omitempty"`
}

// serializeField serialisiert ein String-Feld im Längenpräfix-Binary-Format.
func serializeField(buf []byte, offset int, s string) int {
	l := len(s)
	binary.BigEndian.PutUint16(buf[offset:], uint16(l))
	offset += 2
	copy(buf[offset:], s)
	return offset + l
}

// deserializeField liest ein längenpräfixiertes String aus einem Byte-Slice.
func deserializeField(data []byte) (string, int, error) {
	if len(data) < 2 {
		return "", 0, fmt.Errorf("gql: field too short for length prefix")
	}
	l := int(binary.BigEndian.Uint16(data[:2]))
	if len(data) < 2+l {
		return "", 0, fmt.Errorf("gql: field truncated: need %d bytes, have %d", 2+l, len(data))
	}
	return string(data[2 : 2+l]), 2 + l, nil
}

// fieldSize gibt die serialisierte Größe eines String-Feldes zurück (2 + len(s)).
func fieldSize(s string) int { return 2 + len(s) }

// Encode serialisiert ein GQLQuery in einen Binary-Payload.
//
// Binary-Layout (alle Multi-Byte-Ints sind Big-Endian):
//
//	[0:1]   field_count    uint8   (Anzahl der Felder in der Fields-Map)
//	[1:3]   operation_len  uint16
//	[3:n]   operation      bytes
//	[n:n+2] namespace_len  uint16
//	[n+2:m] namespace      bytes
//	... (entry_id, category, title)
//	[q:r]   field entries  (jeweils: key_len(2) + key + value_len(2) + value)
//	[r:r+4] limit          uint32
//	[s:s+4] offset         uint32
//	[t:t+2] creds_len      uint16
//	[t+2:u] credentials    bytes
func (q *GQLQuery) Encode() []byte {
	size := 1
	size += fieldSize(string(q.Operation))
	size += fieldSize(q.Namespace)
	size += fieldSize(q.EntryID)
	size += fieldSize(q.Category)
	size += fieldSize(q.Title)
	for k, v := range q.Fields {
		size += fieldSize(k) + fieldSize(v)
	}
	size += 4
	size += 4
	size += fieldSize(string(q.Credentials))

	buf := make([]byte, size)
	off := 0

	fc := uint8(len(q.Fields))
	if fc > MaxFieldsCount {
		fc = MaxFieldsCount
	}
	buf[off] = fc
	off++

	off = serializeField(buf, off, string(q.Operation))
	off = serializeField(buf, off, q.Namespace)
	off = serializeField(buf, off, q.EntryID)
	off = serializeField(buf, off, q.Category)
	off = serializeField(buf, off, q.Title)

	for k, v := range q.Fields {
		off = serializeField(buf, off, k)
		off = serializeField(buf, off, v)
	}

	binary.BigEndian.PutUint32(buf[off:], q.Limit)
	off += 4
	binary.BigEndian.PutUint32(buf[off:], q.Offset)
	off += 4
	serializeField(buf, off, string(q.Credentials))

	return buf
}

// DecodeQuery deserialisiert ein GQLQuery aus einem Binary-Payload.
func DecodeQuery(payload []byte, op Operation) (*GQLQuery, error) {
	if len(payload) < 1 {
		return nil, fmt.Errorf("gql: payload too short for field_count")
	}

	off := 0
	fieldCount := int(payload[off])
	off++
	if fieldCount > MaxFieldsCount {
		return nil, fmt.Errorf("gql: field_count %d exceeds max %d", fieldCount, MaxFieldsCount)
	}

	q := &GQLQuery{
		Operation: op,
		Fields:    make(map[string]string),
	}

	s, n, err := deserializeField(payload[off:])
	if err != nil {
		return nil, fmt.Errorf("gql: operation: %w", err)
	}
	if s != "" {
		q.Operation = Operation(s)
	}
	off += n

	s, n, err = deserializeField(payload[off:])
	if err != nil {
		return nil, fmt.Errorf("gql: namespace: %w", err)
	}
	q.Namespace = s
	off += n

	s, n, err = deserializeField(payload[off:])
	if err != nil {
		return nil, fmt.Errorf("gql: entry_id: %w", err)
	}
	q.EntryID = s
	off += n

	s, n, err = deserializeField(payload[off:])
	if err != nil {
		return nil, fmt.Errorf("gql: category: %w", err)
	}
	q.Category = s
	off += n

	s, n, err = deserializeField(payload[off:])
	if err != nil {
		return nil, fmt.Errorf("gql: title: %w", err)
	}
	q.Title = s
	off += n

	for i := 0; i < fieldCount; i++ {
		if off >= len(payload) {
			return nil, fmt.Errorf("gql: truncated at field %d", i)
		}
		k, kn, kerr := deserializeField(payload[off:])
		if kerr != nil {
			return nil, fmt.Errorf("gql: field[%d].key: %w", i, kerr)
		}
		off += kn

		if off >= len(payload) {
			return nil, fmt.Errorf("gql: truncated at field[%d].value", i)
		}
		v, vn, verr := deserializeField(payload[off:])
		if verr != nil {
			return nil, fmt.Errorf("gql: field[%d].value: %w", i, verr)
		}
		off += vn

		q.Fields[k] = v
	}

	if off+4 > len(payload) {
		return nil, fmt.Errorf("gql: truncated at limit")
	}
	q.Limit = binary.BigEndian.Uint32(payload[off:])
	off += 4

	if off+4 > len(payload) {
		return nil, fmt.Errorf("gql: truncated at offset")
	}
	q.Offset = binary.BigEndian.Uint32(payload[off:])
	off += 4

	if off < len(payload) {
		s, n, err = deserializeField(payload[off:])
		if err != nil {
			return nil, fmt.Errorf("gql: credentials: %w", err)
		}
		q.Credentials = []byte(s)
		off += n
	}

	return q, nil
}

// EncodeResult serialisiert ein GQLResult in ein JSON-Byte-Slice für den Wire-Transport.
// Results nutzen JSON für einfache Frontend-Konsumption; Queries nutzen Binary für Security.
func (r *GQLResult) EncodeResult() ([]byte, error) {
	return json.Marshal(r)
}
