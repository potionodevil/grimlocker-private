use hkdf::Hkdf;
use serde::{Deserialize, Serialize};
use sha2::Sha256;
use zeroize::{Zeroize, ZeroizeOnDrop};

use crate::Error;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct Coordinate {
    pub block: usize,
    pub line: usize,
    pub char_index: usize,
}

pub const PANIC_COORDINATE: Coordinate = Coordinate {
    block: 0,
    line: 0,
    char_index: 0,
};

#[derive(Zeroize, ZeroizeOnDrop)]
pub struct DerivedKey(pub Vec<u8>);

pub enum CoordinateResult {
    DerivedKey(DerivedKey),
    PanicTrigger,
}

pub fn parse_coordinates(
    entropy_file: &[u8],
    coords: &[Coordinate],
) -> Result<CoordinateResult, Error> {
    if coords.contains(&PANIC_COORDINATE) {
        return Ok(CoordinateResult::PanicTrigger);
    }

    if coords.is_empty() {
        return Err(Error::Coordinates("no coordinates provided".into()));
    }

    let mut extracted = Vec::with_capacity(coords.len());

    for coord in coords {
        let byte = extract_byte(entropy_file, coord)?;
        extracted.push(byte);
    }

    let key = derive_key(&extracted)?;

    Ok(CoordinateResult::DerivedKey(DerivedKey(key)))
}

fn extract_byte(entropy_file: &[u8], coord: &Coordinate) -> Result<u8, Error> {
    if entropy_file.is_empty() {
        return Err(Error::Coordinates("entropy file is empty".into()));
    }

    let current_block: usize = 0;
    let mut current_line: usize = 0;
    let mut current_char: usize = 0;
    let mut in_newline = false;

    for (_i, &byte) in entropy_file.iter().enumerate() {
        if in_newline {
            in_newline = false;
            current_line += 1;
            current_char = 0;
            if byte == b'\r' {
                continue;
            }
        }

        if byte == b'\n' || byte == b'\r' {
            if current_block == coord.block
                && current_line == coord.line
                && current_char == coord.char_index
            {
                return Ok(byte);
            }
            current_char = 0;
            in_newline = true;
            continue;
        }

        if current_block == coord.block
            && current_line == coord.line
            && current_char == coord.char_index
        {
            return Ok(byte);
        }

        current_char += 1;
    }

    Err(Error::Coordinates(format!(
        "coordinate out of bounds: block={}, line={}, char={}",
        coord.block, coord.line, coord.char_index
    )))
}

fn derive_key(extracted: &[u8]) -> Result<Vec<u8>, Error> {
    let blake3_hash = blake3::hash(extracted);
    let ikm = blake3_hash.as_bytes();

    let salt = b"grimlocker-coordinate-salt-v1";
    let info = b"grimlocker-stage2-override-key";

    let hk = Hkdf::<Sha256>::new(Some(salt), ikm);
    let mut okm = vec![0u8; 32];

    hk.expand(info, &mut okm)
        .map_err(|e| Error::KeyDerivation(format!("HKDF expand failed: {}", e)))?;

    Ok(okm)
}

/// derive_key_from_extracted takes raw extracted bytes and runs BLAKE3 → HKDF-SHA256.
/// This is the same as derive_key() but public, for use by the CGO bridge.
pub fn derive_key_from_extracted(extracted: &[u8]) -> Result<Vec<u8>, Error> {
    derive_key(extracted)
}

/// derive_coordinate_offsets takes an argon2id hash and file size, and produces
/// 32 file offsets using HKDF-SHA256. This matches Go's DeriveCoordinateOffsets.
pub fn derive_coordinate_offsets(argon_hash: &[u8], file_size: i64) -> Result<[i64; 32], Error> {
    if file_size <= 0 {
        return Err(Error::Coordinates("invalid file size".into()));
    }

    let hk = Hkdf::<Sha256>::new(None, argon_hash);
    let mut buf = [0u8; 128];
    hk.expand(b"grimlocker-coordinates-v1", &mut buf)
        .map_err(|e| Error::KeyDerivation(format!("HKDF expand failed: {}", e)))?;

    let mut offsets = [0i64; 32];
    for i in 0..32 {
        let val = u32::from_be_bytes([buf[i * 4], buf[i * 4 + 1], buf[i * 4 + 2], buf[i * 4 + 3]]);
        offsets[i] = (val as i64) % file_size;
    }

    Ok(offsets)
}

/// Derives a workspace-specific key from a master key using HKDF-SHA256.
/// The workspace_id is used as the info parameter, allowing per-workspace keys.
pub fn derive_workspace_key(master_key: &[u8], workspace_id: &str) -> Result<[u8; 32], Error> {
    let salt = b"grimlocker-workspace-v1";
    let hk = Hkdf::<Sha256>::new(Some(salt), master_key);
    let mut okm = [0u8; 32];

    hk.expand(workspace_id.as_bytes(), &mut okm)
        .map_err(|e| Error::KeyDerivation(format!("workspace HKDF expand: {}", e)))?;

    Ok(okm)
}

pub fn parse_coordinate_input(input: &str) -> Result<Vec<Coordinate>, Error> {
    let mut coords = Vec::new();

    for line in input.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }

        let parts: Vec<&str> = line.split(',').collect();
        if parts.len() != 3 {
            return Err(Error::Coordinates(format!(
                "invalid coordinate format '{}': expected block,line,char",
                line
            )));
        }

        let block: usize = parts[0]
            .trim()
            .parse()
            .map_err(|e| Error::Coordinates(format!("parse block: {}", e)))?;

        let line_num: usize = parts[1]
            .trim()
            .parse()
            .map_err(|e| Error::Coordinates(format!("parse line: {}", e)))?;

        let char_idx: usize = parts[2]
            .trim()
            .parse()
            .map_err(|e| Error::Coordinates(format!("parse char_index: {}", e)))?;

        coords.push(Coordinate {
            block,
            line: line_num,
            char_index: char_idx,
        });
    }

    if coords.is_empty() {
        return Err(Error::Coordinates("no valid coordinates found".into()));
    }

    Ok(coords)
}
