import { useState, useEffect, useCallback } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useGrimStore } from './store/useGrimStore'
import { tauriBridge } from './services/tauriBridge'
import { VaultDashboard } from './components/dashboard/VaultDashboard'
import { TerminalError } from './components/shared/TerminalError'
import { SetupScreen } from './components/auth/SetupScreen'
import { LoginScreen } from './components/auth/LoginScreen'
import { SplashScreen } from './components/shared/SplashScreen'
import { AuthProvider, useAuth, AUTH_STATE } from './context/AuthContext'
import { useWindowClose } from './hooks/useWindowClose'
import { useAutofill } from './hooks/useAutofill'
import { AutofillLockedOverlay } from './components/autofill/AutofillLockedOverlay'

const pageVariants = {
  initial: { opacity: 0, scale: 0.98, filter: 'blur(4px)' },
  animate: {
    opacity: 1,
    scale: 1,
    filter: 'blur(0px)',
    transition: { duration: 0.35, ease: 'easeOut' },
  },
  exit: {
    opacity: 0,
    scale: 1.01,
    filter: 'blur(4px)',
    transition: { duration: 0.2 },
  },
}

function AuthErrorScreen({ error, onRetry }) {
  return (
    <div style={{
      position: 'fixed', inset: 0,
      background: 'var(--surface-app, #0f0f11)',
      display: 'flex', flexDirection: 'column',
      alignItems: 'center', justifyContent: 'center',
      gap: 0,
    }}>
      {/* Sad face */}
      <div style={{ fontSize: 72, lineHeight: 1, marginBottom: 24, userSelect: 'none' }}>
        😔
      </div>

      <p style={{
        fontSize: 20, fontWeight: 700,
        color: 'var(--text-primary, #f0f0f0)',
        margin: '0 0 10px',
        letterSpacing: '-0.01em',
      }}>
        Da ist etwas schiefgelaufen
      </p>

      <p style={{
        fontSize: 13,
        color: 'var(--text-tertiary, #888)',
        margin: '0 0 32px',
        maxWidth: 320, textAlign: 'center', lineHeight: 1.5,
      }}>
        Der Vault-Daemon konnte nicht gestartet werden.{error ? ` (${error})` : ''}
      </p>

      <button
        onClick={onRetry}
        style={{
          padding: '10px 28px',
          borderRadius: 8,
          border: '1px solid var(--accent, #4F8EF7)',
          background: 'transparent',
          color: 'var(--accent, #4F8EF7)',
          fontSize: 14, fontWeight: 600,
          cursor: 'pointer',
          transition: 'background 0.15s',
          marginBottom: 12,
        }}
        onMouseEnter={e => e.currentTarget.style.background = 'color-mix(in srgb, var(--accent, #4F8EF7) 12%, transparent)'}
        onMouseLeave={e => e.currentTarget.style.background = 'transparent'}
      >
        Erneut versuchen
      </button>

      <button
        onClick={() => window.location.reload()}
        style={{
          padding: '8px 20px',
          borderRadius: 8,
          border: '1px solid var(--border-default, #2a2a2e)',
          background: 'transparent',
          color: 'var(--text-tertiary, #888)',
          fontSize: 13, cursor: 'pointer',
          transition: 'border-color 0.15s',
        }}
        onMouseEnter={e => e.currentTarget.style.borderColor = 'var(--text-tertiary, #888)'}
        onMouseLeave={e => e.currentTarget.style.borderColor = 'var(--border-default, #2a2a2e)'}
      >
        App neu laden
      </button>
    </div>
  )
}

const MIN_SPLASH_MS = 1400

function AppContent() {
  const { authState, error: authError, retryCheck } = useAuth()
  const { error, setError, setConnected, initPreferences, loadWorkspaces } = useGrimStore()
  const { showLocked, handleUnlock, handleCancelLocked } = useAutofill()
  const [splashDone, setSplashDone] = useState(false)
  useWindowClose()

  useEffect(() => { initPreferences() }, [initPreferences])

  useEffect(() => {
    const t = setTimeout(() => setSplashDone(true), MIN_SPLASH_MS)
    return () => clearTimeout(t)
  }, [])

  const attemptConnect = useCallback(async () => {
    try {
      await tauriBridge.connect()
      // Das 'connected'-Event feuert asynchron via tauriBridge.on('connected') — hier nur initialer Versuch
    } catch (err) {
      console.warn('[App] Initial connection failed:', err.message)
      setConnected(false)
    }
  }, [setConnected])

  useEffect(() => {
    attemptConnect()

    const unsubConnected = tauriBridge.on('connected', () => {
      setConnected(true)
      loadWorkspaces()
    })

    const unsubDisconnected = tauriBridge.on('disconnected', () => {
      setConnected(false)
    })

    const unsubError = tauriBridge.onError((msg) => {
      setError(msg)
    })

    return () => {
      unsubConnected()
      unsubDisconnected()
      unsubError()
    }
  }, [attemptConnect, setError, setConnected, loadWorkspaces])

  const renderView = () => {
    if (authState === AUTH_STATE.CHECKING || !splashDone) {
      return <SplashScreen />
    }

    if (authState === AUTH_STATE.SETUP) {
      return <SetupScreen />
    }

    if (authState === AUTH_STATE.LOGIN) {
      return <LoginScreen />
    }

    if (authState === AUTH_STATE.VAULT) {
      return <VaultDashboard />
    }

    if (authState === AUTH_STATE.ERROR) {
      return <AuthErrorScreen error={authError} onRetry={retryCheck} />
    }

    return <SetupScreen />
  }

  return (
    <div className="w-full h-full">
      <TerminalError message={error} onDismiss={() => setError(null)} />
      <AnimatePresence mode="wait">
        <motion.div
          key={authState}
          variants={pageVariants}
          initial="initial"
          animate="animate"
          exit="exit"
          className="w-full h-full"
        >
          {renderView()}
        </motion.div>
      </AnimatePresence>
      {showLocked && (
        <AutofillLockedOverlay onUnlock={handleUnlock} onCancel={handleCancelLocked} />
      )}
    </div>
  )
}

export default function App() {
  return (
    <AuthProvider>
      <AppContent />
    </AuthProvider>
  )
}
