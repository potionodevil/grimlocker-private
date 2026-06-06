import { useState, useEffect, useCallback } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useGrimStore } from './store/useGrimStore'
import { tauriBridge } from './services/tauriBridge'
import { VaultDashboard } from './components/dashboard/VaultDashboard'
import { TerminalError } from './components/shared/TerminalError'
import { SetupScreen } from './components/auth/SetupScreen'
import { LoginScreen } from './components/auth/LoginScreen'
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

function AuthErrorScreen({ error, retryCount, onRetry }) {
  const [retrying, setRetrying] = useState(false)

  const handleRetry = async () => {
    setRetrying(true)
    onRetry()
  }

  return (
    <div className="min-h-screen bg-surface-app flex items-center justify-center p-6">
      <div className="max-w-md w-full border border-danger/40 rounded-lg p-6 bg-danger-subtle/50">
        <div className="flex items-center gap-3 mb-4">
          <svg className="w-5 h-5 text-danger shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
          </svg>
          <p className="text-danger text-sm font-semibold">Authentication Error</p>
        </div>
        {error && (
          <p className="text-text-secondary text-xs font-mono mb-4 break-all">{error}</p>
        )}
        {retryCount > 0 && (
          <p className="text-text-tertiary text-xs mb-4">{retryCount} attempt{retryCount > 1 ? 's' : ''} made</p>
        )}
        <button
          onClick={handleRetry}
          disabled={retrying}
          className="w-full py-2 px-4 rounded-md text-sm font-medium bg-danger/20 text-danger border border-danger/30 hover:bg-danger/30 transition-colors disabled:opacity-50"
        >
          {retrying ? 'Retrying...' : 'Retry Connection'}
        </button>
      </div>
    </div>
  )
}

function AppContent() {
  const { authState, error: authError, retryCheck, retryCount } = useAuth()
  const { error, setError, setConnected, initPreferences, loadWorkspaces } = useGrimStore()
  const { showLocked, handleUnlock, handleCancelLocked } = useAutofill()
  useWindowClose()

  useEffect(() => { initPreferences() }, [initPreferences])

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
    if (authState === AUTH_STATE.CHECKING) {
      return (
        <div className="min-h-screen bg-surface-app flex items-center justify-center">
          <div className="text-text-secondary text-sm">Initializing vault...</div>
        </div>
      )
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
      return <AuthErrorScreen error={authError} retryCount={retryCount} onRetry={retryCheck} />
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
