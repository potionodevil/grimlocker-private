export const DESIGN_PRESETS = {
  meridian: {
    label: 'Meridian',
    description: 'Klassisch, clean, professionell',
    theme: 'light',
    accentHint: '#0055FF',
    bgHint: '#FBFBFB',
    cardHint: '#FFFFFF',
    textHint: '#1A1A1A',
  },
  obsidian: {
    label: 'Obsidian',
    description: 'Dunkel, technisch, monospace',
    theme: 'dark',
    fontFamily: "'JetBrains Mono', 'Fira Code', ui-monospace, monospace",
    accentHint: '#3B82F6',
    bgHint: '#111113',
    cardHint: '#18181B',
    textHint: '#FAFAFA',
  },
  frost: {
    label: 'Frost',
    description: 'Hell, verspielt, Glassmorphism',
    theme: 'light',
    accentHint: '#0055FF',
    bgHint: '#EFF6FF',
    cardHint: 'rgba(255,255,255,0.72)',
    textHint: '#1A1A1A',
  },
  carbon: {
    label: 'Carbon',
    description: 'Dunkel, brutal-flat, kein Schnickschnack',
    theme: 'dark',
    accentHint: '#22C55E',
    bgHint: '#0A0A0A',
    cardHint: '#111111',
    textHint: '#E5E5E5',
  },
  sakura: {
    label: 'Sakura',
    description: 'Hell, pastellig, weich gerundet',
    theme: 'light',
    fontFamily: "'Nunito', system-ui, sans-serif",
    accentHint: '#EC4899',
    bgHint: '#FFF0F5',
    cardHint: '#FFFFFF',
    textHint: '#1A1A1A',
  },
}

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

// Presets that define their own accent in CSS — inline style must be removed
// so the CSS cascade from tokens.css takes over (inline > stylesheet).
const PRESET_OWN_ACCENT = new Set(['obsidian', 'frost', 'carbon', 'sakura'])

function applyTokens({ theme, density, fontSize, accentKey, sidebarFontSize, designPreset }) {
  const root = document.documentElement
  root.classList.toggle('dark', theme === 'dark')
  root.setAttribute('data-density', density)
  root.style.setProperty('--font-base', `${fontSize}px`)
  root.style.setProperty('--font-sm',   `${Math.round(fontSize * 0.857)}px`)
  root.style.setProperty('--font-lg',   `${Math.round(fontSize * 1.143)}px`)
  root.style.setProperty('--font-xl',   `${Math.round(fontSize * 1.429)}px`)
  root.style.setProperty('--font-2xl',  `${Math.round(fontSize * 1.714)}px`)
  root.style.setProperty('--sidebar-font-size', `${sidebarFontSize ?? 13}px`)

  if (PRESET_OWN_ACCENT.has(designPreset)) {
    // Remove inline overrides so the CSS [data-preset] rules take effect
    root.style.removeProperty('--accent')
    root.style.removeProperty('--accent-hover')
    root.style.removeProperty('--accent-subtle')
  } else {
    // Meridian (and any future user-accent preset): apply user's chosen color
    const preset = ACCENT_PRESETS[accentKey] ?? ACCENT_PRESETS.blue
    root.style.setProperty('--accent',        preset.accent)
    root.style.setProperty('--accent-hover',  preset.accentHover)
    root.style.setProperty('--accent-subtle', preset.accentSubtle)
  }
}

function applyPresetToDOM(key) {
  const root = document.documentElement
  root.setAttribute('data-preset', key === 'meridian' ? '' : key)
  // If switching TO a preset that owns its accent, wipe inline --accent* so
  // the CSS [data-preset] rule wins (inline style beats stylesheet otherwise).
  if (PRESET_OWN_ACCENT.has(key)) {
    root.style.removeProperty('--accent')
    root.style.removeProperty('--accent-hover')
    root.style.removeProperty('--accent-subtle')
  }
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
    sidebarWidth:         224,
    sidebarFontSize:      13,    // 11–16 px, unabhängig vom globalen fontSize
    // Design-Preset
    designPreset:         'meridian',
    pendingPreset:        null,
    sidebarPosition:      'left',  // 'left' | 'right'
    // Vault-Ansicht
    vaultViewMode:        'grid',  // 'grid' | 'list' | 'compact'
    vaultSortBy:          'name',  // 'name' | 'updated' | 'created' | 'type' | 'strength'
    vaultSortDir:         'asc',   // 'asc' | 'desc'
    gridCardSize:         'default', // 'small' | 'default' | 'large'
    showEntrySubtitle:    true,
    showEntryTimestamp:   true,
    // Sicherheit
    autoLockMinutes:      15,
    clipboardClearSeconds: 30,
    showPasswordStrength: true,
    lockdownThreshold:    3,
    // Verhalten
    closeBehavior:        'quit',
    startupView:          'all',
    confirmDelete:        true,
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
    setSidebarFontSize: (v) => {
      update('sidebarFontSize', v)
      applyTokens({ ...get().preferences, sidebarFontSize: v })
    },
    setDesignPreset: (preset) => {
      const def = DESIGN_PRESETS[preset]
      if (!def) return
      // Apply immediately — no restart required
      applyPresetToDOM(preset)
      const prefs = get().preferences
      const newTheme = def.theme
      update('designPreset', preset)
      update('pendingPreset', null)
      update('theme', newTheme)
      applyTokens({ ...prefs, theme: newTheme, designPreset: preset })
    },
    // kept for API compatibility
    applyPendingPreset: () => {},
    setVaultViewMode: (v) => update('vaultViewMode', v),
    setVaultSortBy:   (v) => update('vaultSortBy', v),
    setVaultSortDir:  (v) => update('vaultSortDir', v),
    setGridCardSize:  (v) => update('gridCardSize', v),
    setShowEntrySubtitle:  (v) => update('showEntrySubtitle', v),
    setShowEntryTimestamp: (v) => update('showEntryTimestamp', v),
    setAutoLockMinutes: (v) => update('autoLockMinutes', v),
    setClipboardClearSeconds: (v) => update('clipboardClearSeconds', v),
    setShowPasswordStrength: (v) => update('showPasswordStrength', v),
    setLockdownThreshold: (v) => update('lockdownThreshold', v),
    setCloseBehavior: (v) => update('closeBehavior', v),
    setStartupView: (v) => update('startupView', v),
    setConfirmDelete: (v) => update('confirmDelete', v),
    setSidebarPosition: (v) => update('sidebarPosition', v),

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
      const prefs = get().preferences
      applyPresetToDOM(prefs.designPreset ?? 'meridian')
      applyTokens(prefs)
    },
  }
}
