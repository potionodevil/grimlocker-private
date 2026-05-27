import { create } from 'zustand'
import { createPreferencesSlice } from './preferencesSlice'
import { tauriBridge } from '../services/tauriBridge'

export const useGrimStore = create((set, get) => ({
  ...createPreferencesSlice(set, get),
  view: 'dashboard',
  connected: false,
  error: null,

  header: {
    failedAttempts: 0,
    lockdownTimestamp: 0,
    overrideAttemptsLeft: 4,
    monotonicBootTicks: 0,
    wallclockLastSeen: 0,
  },

  secrets: [],
  activeSecret: null,
  zeroizeProgress: 0,

  entropyInfo: {
    fileSize: 0,
    bitsOfSecurity: 256,
    overrideAttemptsLeft: 4,
  },

  isLockdown: false,
  isCritical: false,

  throughputData: [],
  operationsLog: [],
  daemonStatus: 'offline',

  entries: [],
  activeEntry: null,
  decryptedEntries: {},
  terminalLog: [],
  terminalOpen: false,
  autoLockMinutes: 15,
  lockdownThreshold: 3,

  workspaces: [],
  activeWorkspace: null,
  ipcLog: [],

  setView: (view) => set({ view }),
  setError: (error) => set({ error }),

  setConnected: (connected) => set({ connected }),

  setEntries: (entries) => set({ entries }),
  setActiveEntry: (activeEntry) => set({ activeEntry }),
  clearActiveEntry: () => set({ activeEntry: null }),

  fetchEntries: async () => {
    try {
      const entries = await tauriBridge.listEntries()
      set({ entries })
    } catch (err) {
      console.error('[store] fetchEntries failed:', err.message)
    }
  },

  fetchEntry: async (id) => {
    try {
      const entry = await tauriBridge.getEntry(id)
      set({ activeEntry: entry })
    } catch (err) {
      console.error('[store] fetchEntry failed:', err.message)
    }
  },

  deleteEntryFromStore: async (id) => {
    try {
      await tauriBridge.deleteEntry(id)
      const { entries } = get()
      set({ entries: entries.filter((e) => e.id !== id) })
    } catch (err) {
      console.error('[store] deleteEntry failed:', err.message)
    }
  },

  decryptEntry: async (id) => {
    try {
      const fullEntry = await tauriBridge.decryptEntry(id)
      set({ decryptedEntries: { ...get().decryptedEntries, [id]: fullEntry } })
      return fullEntry
    } catch (err) {
      console.error('[store] decryptEntry failed:', err.message)
      return null
    }
  },

  lockEntry: (id) => {
    const { decryptedEntries } = get()
    const updated = { ...decryptedEntries }
    delete updated[id]
    set({ decryptedEntries: updated })
  },

  lockAllEntries: () => {
    set({ decryptedEntries: {} })
  },

  addTerminalLine: (line) => {
    const state = get()
    set({
      terminalLog: [...state.terminalLog, line].slice(-50),
    })
  },

  setTerminalOpen: (open) => set({ terminalOpen: open }),
  setAutoLockMinutes: (minutes) => set({ autoLockMinutes: minutes }),
  setLockdownThreshold: (threshold) => set({ lockdownThreshold: threshold }),

  setWorkspaces: (workspaces) => set({ workspaces }),
  setActiveWorkspace: (workspace) => set({ activeWorkspace: workspace }),

  addIpcLogEntry: (entry) => {
    const state = get()
    set({
      ipcLog: [entry, ...state.ipcLog].slice(0, 30),
    })
  },

  setHeader: (header) => {
    const isLockdown = header.failedAttempts >= 3
    const isCritical = header.overrideAttemptsLeft === 0

    set({
      header,
      isLockdown,
      isCritical,
      entropyInfo: {
        ...get().entropyInfo,
        overrideAttemptsLeft: header.overrideAttemptsLeft,
        fileSize: get().entropyInfo.fileSize || 0,
      },
    })
  },

  setSecrets: (secrets) => set({ secrets }),

  selectSecret: (secret) => {
    set({ activeSecret: secret, zeroizeProgress: 100 })
    startZeroizeTimer()
  },

  clearActiveSecret: () => {
    set({ activeSecret: null, zeroizeProgress: 0 })
  },

  addSecret: (secret) => {
    const { secrets } = get()
    const updated = [...secrets, { ...secret, id: Date.now().toString(36) }]
    set({ secrets: updated })
  },

  addThroughputPoint: (bytes) => {
    const state = get()
    set({
      throughputData: [...state.throughputData.slice(-29), { t: Date.now(), bytes }],
    })
  },

  addOperation: (type, detail) => {
    const state = get()
    set({
      operationsLog: [{ time: Date.now(), type, detail }, ...state.operationsLog.slice(0, 49)],
    })
  },

  setDaemonStatus: (status) => set({ daemonStatus: status }),
}))

let zeroizeInterval = null

function startZeroizeTimer() {
  if (zeroizeInterval) clearInterval(zeroizeInterval)

  const duration = 30000
  const step = 100
  const interval = duration / (100 / step)

  zeroizeInterval = setInterval(() => {
    const { zeroizeProgress } = useGrimStore.getState()
    const next = Math.max(0, zeroizeProgress - step)
    useGrimStore.setState({ zeroizeProgress: next })

    if (next <= 0) {
      clearInterval(zeroizeInterval)
      useGrimStore.getState().clearActiveSecret()
    }
  }, interval)
}
