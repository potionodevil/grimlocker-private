import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { AutofillPopup } from './components/autofill/AutofillPopup'
import { useGrimStore } from './store/useGrimStore'
import { DESIGN_PRESETS } from './store/preferencesSlice'
import './styles/globals.css'

// Design-Preset VOR dem ersten Render anwenden — verhindert Flash
function applyDesignPreset() {
  try {
    const raw = localStorage.getItem('grimlocker_prefs')
    const saved = raw ? JSON.parse(raw) : {}
    const preset = saved.designPreset ?? 'meridian'
    const presetDef = DESIGN_PRESETS[preset]
    if (!presetDef) return

    document.documentElement.setAttribute('data-preset', preset)

    // Forced theme (hell/dunkel) vom Preset übernehmen
    document.documentElement.classList.toggle('dark', presetDef.theme === 'dark')

    // Font-family wenn das Preset eine spezifische Schrift verlangt
    if (presetDef.fontFamily) {
      document.body.style.fontFamily = presetDef.fontFamily
    }
  } catch { /* localStorage nicht verfügbar */ }
}

const urlParams = new URLSearchParams(window.location.search)

if (urlParams.get('popup') === 'autofill') {
  // Autofill-Popup läuft im selben Vite-Bundle → voller @tauri-apps/api-Zugriff
  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <AutofillPopup />
    </React.StrictMode>
  )
} else {
  applyDesignPreset()
  // Preferences vor dem ersten React-Render anwenden (kein weisser Flash bei Dark Mode)
  useGrimStore.getState().initPreferences()
  import('./styles/autofill.css')

  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>
  )
}
