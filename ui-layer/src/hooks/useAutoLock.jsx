import { useEffect, useRef, useCallback } from 'react'
import { useGrimStore } from '../store/useGrimStore'
import { tauriBridge } from '../services/tauriBridge'

export function useAutoLock(isUnlocked) {
  const autoLockMinutes = useGrimStore((s) => s.preferences.autoLockMinutes ?? 15)
  const timerRef = useRef(null)

  const resetTimer = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    if (!isUnlocked) return
    if (autoLockMinutes === 0) return  // 0 = nie sperren — der User hat's so eingestellt

    timerRef.current = setTimeout(() => {
      console.log('[AutoLock] Inactivity timeout — clearing session key')
      tauriBridge.clearSessionKey()
      useGrimStore.getState().lockAllEntries()
    }, autoLockMinutes * 60 * 1000)
  }, [isUnlocked, autoLockMinutes])

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