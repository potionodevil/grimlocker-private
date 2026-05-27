import { useEffect, useRef, useCallback } from 'react'
import { useGrimStore } from '../store/useGrimStore'
import { tauriBridge } from '../services/tauriBridge'

const AUTO_LOCK_MINUTES = 15

export function useAutoLock(isUnlocked) {
  const timerRef = useRef(null)

  const resetTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
    }
    if (!isUnlocked) return

    timerRef.current = setTimeout(() => {
      console.log('[AutoLock] Inactivity timeout — clearing session key')
      tauriBridge.clearSessionKey()
      useGrimStore.getState().lockAllEntries()
    }, AUTO_LOCK_MINUTES * 60 * 1000)
  }, [isUnlocked])

  useEffect(() => {
    if (!isUnlocked) {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
      return
    }

    resetTimer()

    const events = ['mousedown', 'keydown', 'scroll', 'touchstart', 'click']
    const handler = () => resetTimer()

    for (const event of events) {
      window.addEventListener(event, handler, { passive: true })
    }

    return () => {
      if (timerRef.current) {
        clearTimeout(timerRef.current)
        timerRef.current = null
      }
      for (const event of events) {
        window.removeEventListener(event, handler)
      }
    }
  }, [isUnlocked, resetTimer])
}