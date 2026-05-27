import { useState, useEffect, useRef, useCallback } from 'react'

export function useCountdown(lockdownTimestamp, lockdownMinutes = 200) {
  const [timeLeft, setTimeLeft] = useState(0)
  const [isExpired, setIsExpired] = useState(false)
  const intervalRef = useRef(null)

  const calculateTimeLeft = useCallback(() => {
    if (!lockdownTimestamp || lockdownTimestamp === 0) {
      return 0
    }

    const now = Math.floor(Date.now() / 1000)
    const expiry = lockdownTimestamp + (lockdownMinutes * 60)
    const remaining = expiry - now

    return Math.max(0, remaining)
  }, [lockdownTimestamp, lockdownMinutes])

  useEffect(() => {
    const initial = calculateTimeLeft()
    setTimeLeft(initial)
    setIsExpired(initial <= 0)

    if (initial > 0) {
      intervalRef.current = setInterval(() => {
        const remaining = calculateTimeLeft()
        setTimeLeft(remaining)
        setIsExpired(remaining <= 0)

        if (remaining <= 0) {
          clearInterval(intervalRef.current)
        }
      }, 1000)
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
      }
    }
  }, [lockdownTimestamp, calculateTimeLeft])

  const minutes = Math.floor(timeLeft / 60)
  const seconds = timeLeft % 60
  const hours = Math.floor(minutes / 60)
  const displayMinutes = minutes % 60

  const formatted = `${String(hours).padStart(2, '0')}:${String(displayMinutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`

  return {
    timeLeft,
    isExpired,
    formatted,
    minutes,
    seconds,
  }
}
