//! GQL binary frame encoder/decoder.
//!
//! Wire format (all integers big-endian):
//!   [0..4]  total_length  u32   (bytes after header)
//!   [4]     opcode        u8    (0x10 query, 0x11 result, 0x12 error)
//!   [5..]   payload
//!
//! Query payload:
//!   [0]     field_count   u8
//!   LP-strings: operation, namespace, entry_id, category, title
//!   field_count × (key LP-string + value LP-string)
//!   [-8..-4] limit   u32
//!   [-4..]   offset  u32
//!
//! LP-string: u16 big-endian length prefix + UTF-8 bytes.

use std::collections::HashMap;
use crate::error::Error;
use serde::{Deserialize, Serialize};

pub const OPCODE_QUERY:  u8 = 0x10;
pub const OPCODE_RESULT: u8 = 0x11;
pub const OPCODE_ERROR:  u8 = 0x12;

fn lp(s: &str) -> Vec<u8> {
    let b = s.as_bytes();
    let len = b.len() as u16;
    let mut out = len.to_be_bytes().to_vec();
    out.extend_from_slice(b);
    out
}

pub fn encode_query(
    operation: &str,
    namespace: &str,
    entry_id:  &str,
    category:  &str,
    title:     &str,
    fields:    &HashMap<String, String>,
    limit:     u32,
    offset:    u32,
) -> Vec<u8> {
    let mut payload = vec![fields.len().min(255) as u8];
    payload.extend(lp(operation));
    payload.extend(lp(namespace));
    payload.extend(lp(entry_id));
    payload.extend(lp(category));
    payload.extend(lp(title));
    for (k, v) in fields {
        payload.extend(lp(k));
        payload.extend(lp(v));
    }
    payload.extend(limit.to_be_bytes());
    payload.extend(offset.to_be_bytes());

    let total_len = (1 + payload.len()) as u32;
    let mut frame = total_len.to_be_bytes().to_vec();
    frame.push(OPCODE_QUERY);
    frame.extend(payload);
    frame
}

pub fn parse_response(frame: &[u8]) -> Result<GqlResponse, Error> {
    if frame.len() < 5 {
        return Err(Error::Protocol(format!("frame too short: {} bytes", frame.len())));
    }
    let opcode = frame[4];
    let payload = &frame[5..];
    let parsed: GqlResponse = serde_json::from_slice(payload)
        .map_err(|e| Error::Protocol(format!("bad JSON payload: {e}")))?;
    if opcode == OPCODE_ERROR || !parsed.success {
        return Err(Error::Daemon {
            code:    crate::error::ErrorCode::from(parsed.error_code.unwrap_or(0)),
            message: parsed.error_msg.clone().unwrap_or_else(|| "unknown error".into()),
        });
    }
    Ok(parsed)
}

#[derive(Debug, Deserialize, Serialize)]
pub struct GqlResponse {
    pub success:    bool,
    pub entries:    Option<Vec<serde_json::Value>>,
    pub error_code: Option<i64>,
    pub error_msg:  Option<String>,
}
