/**
 * crypto.js — Clientseitige ChaCha20-Poly1305-Entschlüsselung via @noble/ciphers.
 *
 * Warum das Ganze? Alle sensitiven Daten (Passwörter, SSH-Keys, Zertifikate) werden
 * vom Daemon mit einem Session-Key (SKE) verschlüsselt, bevor sie über den WebSocket
 * geschickt werden. Das Frontend bekommt also nie Rohdaten zu sehen.
 * Der Session-Key wird beim Unlock abgeleitet und lebt NUR im JS-RAM — kein
 * localStorage, kein Cookie, nirgends. Beim Reload ist er weg.
 *
 * Wire-Format für SKE-Payloads:
 *   base64(nonce[12] + ciphertext_with_tag)
 */
import { chacha20poly1305 } from '@noble/ciphers/chacha.js'

/**
 * Entschlüsselt ein SKE-encrypted Payload.
 * @param {string} encryptedB64 — Base64-kodiert (nonce[12] + ciphertext+tag)
 * @param {Uint8Array} key — 32-Byte Session-Key
 * @returns {string} Entschlüsselter UTF-8-String
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
 * Entschlüsselt ein einzelnes SKE-Feld — tolerant gegenüber Plaintext.
 * Wenn der Wert nie verschlüsselt wurde (z.B. weil das Feld optional ist),
 * geben wir einfach den Originalwert zurück.
 * @param {string} value — Plaintext oder SKE-Base64-Blob
 * @param {Uint8Array} key — 32-Byte Session-Key
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
 * Base64 → Uint8Array. Der Klassiker, nur halt ohne Buffer.
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
 * Uint8Array → Base64. Der Rückweg.
 */
export function bytesToBase64(bytes) {
  let bin = ''
  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i])
  }
  return btoa(bin)
}