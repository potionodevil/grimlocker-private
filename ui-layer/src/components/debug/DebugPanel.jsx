import { useState, useEffect } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import styles from './DebugPanel.module.css'

export const DebugPanel = () => {
  const [password, setPassword] = useState('')
  const [recoveryPhrase, setRecoveryPhrase] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [vaultStatus, setVaultStatus] = useState(null)
  const { header, ipcLog } = useGrimStore()

  useEffect(() => {
    loadVaultStatus()
  }, [])

  const loadVaultStatus = async () => {
    try {
      const status = await tauriBridge.checkVaultStatus()
      setVaultStatus(status)
    } catch (err) {
      setError(`Status check failed: ${err.message}`)
    }
  }

  const handleGetRecoveryPhrase = async (e) => {
    e.preventDefault()
    if (!password) {
      setError('Password required')
      return
    }

    setLoading(true)
    setError('')

    try {
      const phrase = await tauriBridge.getRecoveryPhrase(password)
      setRecoveryPhrase(phrase)
      setPassword('')
    } catch (err) {
      setError(`Failed to retrieve recovery phrase: ${err.message}`)
      setRecoveryPhrase('')
    } finally {
      setLoading(false)
    }
  }

  const handleCopyPhrase = () => {
    if (recoveryPhrase) {
      navigator.clipboard.writeText(recoveryPhrase)
      setError('')
      setTimeout(() => setRecoveryPhrase(''), 2000)
    }
  }

  const handleClearLockdown = async () => {
    setLoading(true)
    setError('')

    try {
      const status = await tauriBridge.checkVaultStatus()
      setVaultStatus(status)
      setError('')
    } catch (err) {
      setError(`Clear lockdown failed: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const handlePanicWipe = async () => {
    if (!window.confirm('⚠️ This will completely wipe the vault. Are you sure?')) {
      return
    }

    setLoading(true)
    setError('')

    try {
      await tauriBridge.panicWipe()
      setError('Vault wiped successfully. Reinitialize to use.')
      setRecoveryPhrase('')
    } catch (err) {
      setError(`Panic wipe failed: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.container}>
      <div className={styles.header}>
        <h2>⚠️ DEBUG MODE — ENTFERNEN VOR RELEASE</h2>
      </div>

      <section className={styles.section}>
        <h3>VAULT STATUS</h3>
        {vaultStatus && (
          <div className={styles.status}>
            <div className={styles.statusItem}>
              <span>initialized:</span>
              <strong>{vaultStatus.initialized ? 'true' : 'false'}</strong>
            </div>
            <div className={styles.statusItem}>
              <span>unlocked:</span>
              <strong>{vaultStatus.unlocked ? 'true' : 'false'}</strong>
            </div>
            <div className={styles.statusItem}>
              <span>port:</span>
              <strong>{tauriBridge.port || 'unknown'}</strong>
            </div>
          </div>
        )}
        {header && (
          <div className={styles.headerInfo}>
            <div className={styles.infoItem}>
              <span>failedAttempts: {header.failedAttempts}</span>
            </div>
            <div className={styles.infoItem}>
              <span>overrideAttemptsLeft: {header.overrideAttemptsLeft}</span>
            </div>
          </div>
        )}
      </section>

      <section className={styles.section}>
        <h3>RECOVERY PHRASE (benötigt Passwort)</h3>
        <form onSubmit={handleGetRecoveryPhrase} className={styles.form}>
          <input
            type="password"
            placeholder="Passwort eingeben"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            disabled={loading}
            className={styles.input}
          />
          <button type="submit" disabled={loading} className={styles.button}>
            {loading ? 'Laden...' : 'Phrase anzeigen'}
          </button>
        </form>

        {recoveryPhrase && (
          <div className={styles.phraseContainer}>
            <textarea
              readOnly
              value={recoveryPhrase}
              className={styles.phrase}
              rows={4}
            />
            <button onClick={handleCopyPhrase} className={styles.button}>
              📋 In Zwischenablage kopieren
            </button>
          </div>
        )}
      </section>

      <section className={styles.section}>
        <h3>RESET OPERATIONS</h3>
        <div className={styles.buttonGroup}>
          <button
            onClick={handleClearLockdown}
            disabled={loading}
            className={styles.button}
          >
            🔓 Clear Lockdown
          </button>
          <button
            onClick={handlePanicWipe}
            disabled={loading}
            className={`${styles.button} ${styles.danger}`}
          >
            💥 Complete Wipe
          </button>
        </div>
      </section>

      <section className={styles.section}>
        <h3>RAW IPC LOG (letzte {Math.min(ipcLog.length, 30)} Nachrichten)</h3>
        <div className={styles.logContainer}>
          {ipcLog.length === 0 ? (
            <div className={styles.emptyLog}>Keine IPC-Nachrichten</div>
          ) : (
            ipcLog.map((entry, idx) => (
              <div key={idx} className={styles.logEntry}>
                <span className={styles.logType}>{entry.type}</span>
                <span className={styles.logDetail}>{entry.detail}</span>
              </div>
            ))
          )}
        </div>
      </section>

      {error && <div className={styles.error}>{error}</div>}
    </div>
  )
}
