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
    if (raw) return JSON.parse(raw)
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
    // Darstellung
    theme:                'light',
    density:              'cozy',
    fontSize:             14,
    accentKey:            'blue',
    reduceMotion:         false,
    highContrast:         false,
    sidebarWidth:         224,   // Pixel
    // Sicherheit
    autoLockMinutes:      15,
    clipboardClearSeconds: 30,
    showPasswordStrength: true,
    lockdownThreshold:    3,
    // Verhalten
    closeBehavior:        'quit', // 'quit' (beenden) | 'minimize' (minimieren)
    startupView:          'all',  // 'all' | 'passwords' | 'FILE_VAULT' | 'dashboard'
    confirmDelete:        true,
    // Vault-Gruppen — Array von { id, label, color, type }
    passwordGroups:       [],
  }

  const initialPreferences = { ...defaults, ...saved }

  const update = (key, val) => {
    set((s) => {
      const next = { ...s.preferences, [key]: val }
      savePreferencesToStorage(next)
      return { preferences: next }
    })
  }

  return {
    preferences: initialPreferences,

    setTheme: (theme) => {
      update('theme', theme)
      applyTokens({ ...get().preferences, theme })
    },

    setDensity: (density) => {
      update('density', density)
      applyTokens({ ...get().preferences, density })
    },

    setFontSize: (fontSize) => {
      update('fontSize', fontSize)
      applyTokens({ ...get().preferences, fontSize })
    },

    setAccentKey: (accentKey) => {
      update('accentKey', accentKey)
      applyTokens({ ...get().preferences, accentKey })
    },

    setReduceMotion: (v) => update('reduceMotion', v),
    setHighContrast: (v) => update('highContrast', v),
    setSidebarWidth: (v) => update('sidebarWidth', v),
    setAutoLockMinutes: (v) => update('autoLockMinutes', v),
    setClipboardClearSeconds: (v) => update('clipboardClearSeconds', v),
    setShowPasswordStrength: (v) => update('showPasswordStrength', v),
    setLockdownThreshold: (v) => update('lockdownThreshold', v),
    setCloseBehavior: (v) => update('closeBehavior', v),
    setStartupView: (v) => update('startupView', v),
    setConfirmDelete: (v) => update('confirmDelete', v),

    addPasswordGroup: (group) => {
      const groups = [...get().preferences.passwordGroups, group]
      update('passwordGroups', groups)
    },
    removePasswordGroup: (id) => {
      const groups = get().preferences.passwordGroups.filter(g => g.id !== id)
      update('passwordGroups', groups)
    },
    renamePasswordGroup: (id, label) => {
      const groups = get().preferences.passwordGroups.map(g => g.id === id ? { ...g, label } : g)
      update('passwordGroups', groups)
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
