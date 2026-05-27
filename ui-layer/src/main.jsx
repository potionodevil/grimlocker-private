import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { useGrimStore } from './store/useGrimStore'
import './styles/globals.css'

// Apply saved preferences (theme, density, font-size, accent)
// BEFORE React mounts to prevent white flash on dark mode.
useGrimStore.getState().initPreferences()

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
)
