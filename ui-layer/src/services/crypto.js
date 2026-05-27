/**
 * crypto.js — Client-side ChaCha20-Poly1305 decryption using @noble/ciphers.
 *
 * All sensitive data (passwords, SSH keys, certificates) is encrypted
 * with a session key (SKE) before being sent over the WebSocket. The
 * session key is derived at unlock time and held only in JS RAM.
 * It is NEVER persisted and vanishes on page reload.
 *
 * Wire format for SKE-encrypted payloads:
 *   base64(nonce[12] + ciphertext_with_tag)
 */
import { chacha20poly1305 } from '@noble/ciphers/chacha.js'

/**
 * Decrypt an SKE-encrypted payload.
 * @param {string} encryptedB64 — Base64-encoded (nonce[12] + ciphertext+tag)
 * @param {Uint8Array} key — 32-byte session key
 * @returns {string} Decrypted UTF-8 string
 */
export function decryptSKE(encryptedB64, key) {
  const raw = base64ToBytes(encryptedB64)
  if (raw.length < 12) {
    throw new Error('SKE payload too short')
  }
  const nonce = raw.slice(0, 12)
  const ciphertext = raw.slice(12)
  const cipher = chacha20poly1305(key, nonce)
  const plaintext = cipher.decrypt(ciphertext)
  return new TextDecoder().decode(plaintext)
}

/**
 * Decrypt a single SKE-encrypted field value.
 * Returns the plaintext string, or the original value if decryption fails
 * (e.g. the field was never encrypted).
 * @param {string} value — Either a plaintext string or an SKE-encrypted base64 blob
 * @param {Uint8Array} key — 32-byte session key
 * @returns {string}
 */
export function decryptField(value, key) {
  if (!value || !key) return value
  try {
    return decryptSKE(value, key)
  } catch {
    return value
  }
}

/**
 * Decode a base64 string to a Uint8Array.
 */
export function base64ToBytes(b64) {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) {
    bytes[i] = bin.charCodeAt(i)
  }
  return bytes
}

/**
 * Encode a Uint8Array to a base64 string.
 */
export function bytesToBase64(bytes) {
  let bin = ''
  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i])
  }
  return btoa(bin)
}