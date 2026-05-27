const ACCENT_PRESETS = {
  blue:   { accent: '#0055FF', accentHover: '#0044CC', accentSubtle: '#EBF0FF' },
  indigo: { accent: '#4F46E5', accentHover: '#4338CA', accentSubtle: '#EEF2FF' },
  teal:   { accent: '#00A3A3', accentHover: '#008080', accentSubtle: '#E0FAFA' },
  green:  { accent: '#16A34A', accentHover: '#15803D', accentSubtle: '#DCFCE7' },
  purple: { accent: '#7C3AED', accentHover: '#6D28D9', accentSubtle: '#EDE9FE' },
}

export const ACCENT_COLORS = Object.keys(ACCENT_PRESETS)

const STORAGE_KEY = 'grimlocker_prefs'

function loadSavedPreferences() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      return parsed
    }
  } catch (e) {
    console.warn('[preferences] Failed to load saved preferences:', e)
  }
  return null
}

function savePreferencesToStorage(prefs) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch (e) {
    console.warn('[preferences] Failed to save preferences:', e)
  }
}

function applyTokens({ theme, density, fontSize, accentKey }) {
  const root = document.documentElement
  root.classList.toggle('dark', theme === 'dark')
  root.setAttribute('data-density', density)
  root.style.setProperty('--font-base', `${fontSize}px`)
  root.style.setProperty('--font-sm',   `${Math.round(fontSize * 0.857)}px`)
  root.style.setProperty('--font-lg',   `${Math.round(fontSize * 1.143)}px`)
  root.style.setProperty('--font-xl',   `${Math.round(fontSize * 1.429)}px`)
  root.style.setProperty('--font-2xl',  `${Math.round(fontSize * 1.714)}px`)

  const preset = ACCENT_PRESETS[accentKey] ?? ACCENT_PRESETS.blue
  root.style.setProperty('--accent',        preset.accent)
  root.style.setProperty('--accent-hover',  preset.accentHover)
  root.style.setProperty('--accent-subtle', preset.accentSubtle)
}

export const createPreferencesSlice = (set, get) => {
  const saved = loadSavedPreferences()
  const defaults = {
    theme:        'light',
    density:      'cozy',
    fontSize:     14,
    accentKey:    'blue',
    reduceMotion: false,
    highContrast: false,
  }

  const initialPreferences = { ...defaults, ...saved }

  return {
    preferences: initialPreferences,

    setTheme: (theme) => {
      set((s) => {
        const next = { ...s.preferences, theme }
        savePreferencesToStorage(next)
        return { preferences: next }
      })
      applyTokens({ ...get().preferences, theme })
    },

    setDensity: (density) => {
      set((s) => {
        const next = { ...s.preferences, density }
        savePreferencesToStorage(next)
        return { preferences: next }
      })
      applyTokens({ ...get().preferences, density })
    },

    setFontSize: (fontSize) => {
      set((s) => {
        const next = { ...s.preferences, fontSize }
        savePreferencesToStorage(next)
        return { preferences: next }
      })
      applyTokens({ ...get().preferences, fontSize })
    },

    setAccentKey: (accentKey) => {
      set((s) => {
        const next = { ...s.preferences, accentKey }
        savePreferencesToStorage(next)
        return { preferences: next }
      })
      applyTokens({ ...get().preferences, accentKey })
    },

    setReduceMotion: (reduceMotion) => {
      set((s) => {
        const next = { ...s.preferences, reduceMotion }
        savePreferencesToStorage(next)
        return { preferences: next }
      })
    },

    setHighContrast: (highContrast) => {
      set((s) => {
        const next = { ...s.preferences, highContrast }
        savePreferencesToStorage(next)
        return { preferences: next }
      })
    },

    resetPreferences: () => {
      savePreferencesToStorage(defaults)
      set({ preferences: defaults })
      applyTokens(defaults)
    },

    initPreferences: () => {
      applyTokens(get().preferences)
    },
  }
}
