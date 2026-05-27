import { createContext, useContext, useState, useCallback, useEffect, useRef } from 'react'
import { tauriBridge } from '../services/tauriBridge'
import { useGrimStore } from '../store/useGrimStore'
import { useAutoLock } from '../hooks/useAutoLock.jsx'

const AuthContext = createContext(null)

export const AUTH_STATE = {
  CHECKING: 'checking',
  SETUP: 'setup',
  LOGIN: 'login',
  VAULT: 'vault',
  ERROR: 'error',
}

const MAX_AUTO_RETRIES = 5
const BASE_RETRY_DELAY = 1000
const MAX_RETRY_DELAY = 8000

export function AuthProvider({ children }) {
  const [authState, setAuthState] = useState(AUTH_STATE.CHECKING)
  const [isInitialized, setIsInitialized] = useState(false)
  const [isUnlocked, setIsUnlocked] = useState(false)
  const [error, setError] = useState(null)
  const [attemptsRemaining, setAttemptsRemaining] = useState(3)
  const [retryCount, setRetryCount] = useState(0)
  const autoRetryRef = useRef(null)

  const checkVaultStatus = useCallback(async () => {
    try {
      const status = await tauriBridge.checkVaultStatus()
      setIsInitialized(status.initialized)
      setRetryCount(0)
      if (status.initialized && !status.isV5) {
        setAuthState(AUTH_STATE.SETUP)
      } else {
        setAuthState(status.initialized ? AUTH_STATE.LOGIN : AUTH_STATE.SETUP)
      }
    } catch (err) {
      const retryNum = retryCount + 1
      setRetryCount(retryNum)
      if (retryNum < MAX_AUTO_RETRIES) {
        const delay = Math.min(BASE_RETRY_DELAY * Math.pow(2, retryNum - 1), MAX_RETRY_DELAY)
        if (autoRetryRef.current) clearTimeout(autoRetryRef.current)
        autoRetryRef.current = setTimeout(() => {
          checkVaultStatus()
        }, delay)
      } else {
        setAuthState(AUTH_STATE.ERROR)
        setError('Failed to check vault status')
      }
    }
  }, [retryCount])

  const retryCheck = useCallback(() => {
    if (autoRetryRef.current) clearTimeout(autoRetryRef.current)
    setRetryCount(0)
    setError(null)
    setAuthState(AUTH_STATE.CHECKING)
    checkVaultStatus()
  }, [checkVaultStatus])

  useEffect(() => {
    checkVaultStatus()
    return () => {
      if (autoRetryRef.current) clearTimeout(autoRetryRef.current)
    }
  }, [])

  const initializeVault = useCallback(async (password) => {
    try {
      const phrase = await tauriBridge.initializeVault(password)
      if (!phrase) {
        throw new Error('Failed to generate recovery phrase')
      }
      // After init, the daemon auto-unlocks but we still need the session key
      // on the frontend side. Explicitly unlock to establish the SKE key.
      try {
        await tauriBridge.unlockVault(password)
      } catch (unlockErr) {
        console.warn('[Auth] Post-init unlock (for session key) failed:', unlockErr.message)
      }
      setIsInitialized(true)
      useGrimStore.getState().fetchEntries()
      return phrase
    } catch (err) {
      setError(err.message)
      return null
    }
  }, [])

  const unlockVault = useCallback(async (password) => {
    try {
      const result = await tauriBridge.unlockVault(password)
      if (!result.success) {
        throw new Error('Invalid password')
      }
      setIsUnlocked(true)
      setAuthState(AUTH_STATE.VAULT)
      setAttemptsRemaining(3)
      useGrimStore.getState().fetchEntries()
      return true
    } catch (err) {
      const newAttempts = attemptsRemaining - 1
      setAttemptsRemaining(newAttempts)
      if (newAttempts <= 0) {
        setAuthState(AUTH_STATE.ERROR)
        setError('Hard lockdown — too many failed attempts')
      } else {
        setError(err.message)
      }
      return false
    }
  }, [attemptsRemaining])

  const confirmVaultReady = useCallback(() => {
    setIsUnlocked(true)
    setAuthState(AUTH_STATE.VAULT)
  }, [])

  const resetVault = useCallback(async (phrase) => {
    try {
      await tauriBridge.resetVault(phrase)
      setIsInitialized(false)
      setIsUnlocked(false)
      setAuthState(AUTH_STATE.SETUP)
      return true
    } catch (err) {
      setError(err.message)
      return false
    }
  }, [])

  const logout = useCallback(() => {
    setIsUnlocked(false)
    setAuthState(AUTH_STATE.LOGIN)
    tauriBridge.clearSessionKey()
    useGrimStore.getState().lockAllEntries()
  }, [])

  const clearError = useCallback(() => {
    setError(null)
  }, [])

  useAutoLock(isUnlocked)

  const value = {
    authState,
    isInitialized,
    isUnlocked,
    error,
    attemptsRemaining,
    retryCount,
    initializeVault,
    unlockVault,
    confirmVaultReady,
    resetVault,
    logout,
    clearError,
    retryCheck,
  }

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return context
}