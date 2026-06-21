import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { AutofillPopup } from './components/autofill/AutofillPopup'
import { useGrimStore } from './store/useGrimStore'
import './styles/globals.css'

const urlParams = new URLSearchParams(window.location.search)

if (urlParams.get('popup') === 'autofill') {
  // Autofill-Popup läuft im selben Vite-Bundle → voller @tauri-apps/api-Zugriff
  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <AutofillPopup />
    </React.StrictMode>
  )
} else {
  // Preferences vor dem ersten React-Render anwenden (kein weisser Flash bei Dark Mode)
  useGrimStore.getState().initPreferences()
  import('./styles/autofill.css')

  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>
  )
}
