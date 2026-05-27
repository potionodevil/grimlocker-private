/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{js,jsx,ts,tsx}'],
  theme: {
    extend: {
      colors: {
        surface: {
          app:     'var(--surface-app)',
          base:    'var(--surface-base)',
          subtle:  'var(--surface-subtle)',
          overlay: 'var(--surface-overlay)',
        },
        border: {
          DEFAULT: 'var(--border-default)',
          strong:  'var(--border-strong)',
        },
        text: {
          primary:   'var(--text-primary)',
          secondary: 'var(--text-secondary)',
          tertiary:  'var(--text-tertiary)',
          disabled:  'var(--text-disabled)',
          inverse:   'var(--text-inverse)',
        },
        accent:  'var(--accent)',
        'accent-hover':   'var(--accent-hover)',
        'accent-subtle':  'var(--accent-subtle)',
        success:  'var(--success)',
        'success-subtle': 'var(--success-subtle)',
        warning:  'var(--warning)',
        'warning-subtle': 'var(--warning-subtle)',
        danger:   'var(--danger)',
        'danger-subtle':  'var(--danger-subtle)',
      },
      fontFamily: {
        sans: ['"Inter"', '"Geist Sans"', 'system-ui', 'sans-serif'],
      },
      fontSize: {
        sm:    ['var(--font-sm)',   { lineHeight: '1.5' }],
        base:  ['var(--font-base)', { lineHeight: '1.5' }],
        lg:    ['var(--font-lg)',   { lineHeight: '1.4' }],
        xl:    ['var(--font-xl)',   { lineHeight: '1.3' }],
        '2xl': ['var(--font-2xl)', { lineHeight: '1.2' }],
      },
      spacing: {
        'dp-x':   'var(--density-px)',
        'dp-y':   'var(--density-py)',
        'dp-gap': 'var(--density-gap)',
        'dp-row': 'var(--density-row)',
      },
      height: {
        'dp-row': 'var(--density-row)',
      },
      boxShadow: {
        xs: 'var(--shadow-xs)',
        sm: 'var(--shadow-sm)',
        md: 'var(--shadow-md)',
      },
      borderRadius: {
        sm: 'var(--radius-sm)',
        md: 'var(--radius-md)',
        lg: 'var(--radius-lg)',
      },
      transitionTimingFunction: {
        'out-expo': 'cubic-bezier(0.16, 1, 0.3, 1)',
      },
    },
  },
  plugins: [],
}
