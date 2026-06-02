"""
Low-level GQL binary frame encoder/decoder.
Internal — use grimlocker.Client instead.

Wire format (all multi-byte integers big-endian):
  [0:4]  total_length  uint32  (length of everything after this header)
  [4]    opcode        uint8   (0x10 = query, 0x11 = result, 0x12 = error)
  [5:]   payload       bytes

Query payload layout:
  [0]    field_count   uint8
  [1:n]  operation     LP-string (uint16 len + UTF-8 bytes)
  ...    namespace, entry_id, category, title  (each LP-string)
  ...    fields        field_count × (key LP-string + value LP-string)
  [-8:-4] limit        uint32
  [-4:]   offset       uint32
"""

import struct
from typing import Dict

OPCODE_QUERY  = 0x10
OPCODE_RESULT = 0x11
OPCODE_ERROR  = 0x12


def _lp(s: str) -> bytes:
    """Encode a length-prefixed string: uint16 big-endian length + UTF-8 bytes."""
    b = (s or "").encode("utf-8")
    return struct.pack(">H", len(b)) + b


def encode_query(
    operation: str,
    namespace: str,
    entry_id: str,
    category: str,
    title: str,
    fields: Dict[str, str],
    limit: int,
    offset: int,
) -> bytes:
    """Return a wire-ready GQL query frame (4-byte length prefix + opcode + payload)."""
    payload = bytes([min(len(fields), 255)])  # field_count
    payload += _lp(operation)
    payload += _lp(namespace)
    payload += _lp(entry_id)
    payload += _lp(category)
    payload += _lp(title)
    for k, v in (fields or {}).items():
        payload += _lp(k) + _lp(v)
    payload += struct.pack(">II", limit, offset)

    header = struct.pack(">IB", 1 + len(payload), OPCODE_QUERY)
    return header + payload


def read_opcode(frame: bytes) -> int:
    if len(frame) < 5:
        raise ValueError(f"frame too short: {len(frame)} bytes")
    return frame[4]


def read_payload(frame: bytes) -> bytes:
    if len(frame) < 5:
        raise ValueError(f"frame too short: {len(frame)} bytes")
    return frame[5:]
