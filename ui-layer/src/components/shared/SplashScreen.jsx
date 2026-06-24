export function SplashScreen() {
  return (
    <div style={{
      position: 'fixed', inset: 0,
      background: 'var(--surface-app, #0f0f11)',
      display: 'flex', flexDirection: 'column',
      alignItems: 'center', justifyContent: 'center',
      gap: 20,
      zIndex: 9999,
    }}>
      <Spinner />
      <p style={{
        fontFamily: 'ui-monospace, monospace',
        fontSize: 11,
        letterSpacing: '0.14em',
        textTransform: 'uppercase',
        color: 'var(--text-tertiary, #888)',
        margin: 0,
      }}>
        Vault wird geladen...
      </p>
      <style>{`
        @keyframes gl-spin {
          to { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  )
}

function Spinner() {
  return (
    <div style={{
      width: 36, height: 36,
      borderRadius: '50%',
      border: '3px solid var(--border-default, #2a2a2e)',
      borderTopColor: 'var(--accent, #4F8EF7)',
      animation: 'gl-spin 0.75s linear infinite',
    }} />
  )
}
