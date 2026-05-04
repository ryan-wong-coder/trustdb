/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './index.html',
    './src/**/*.{vue,js,ts,jsx,tsx}',
  ],
  darkMode: 'class',
  theme: {
    extend: {
      fontFamily: {
        display: [
          '"Bahnschrift"',
          '"IBM Plex Sans Condensed"',
          '"Arial Narrow"',
          '"Agency FB"',
          'Impact',
          'sans-serif',
        ],
        sans: [
          '"IBM Plex Sans"',
          '"TrustDB Sans"',
          '"Segoe UI Variable"',
          '"Segoe UI"',
          'system-ui',
          'sans-serif',
        ],
        mono: [
          '"Cascadia Code"',
          '"IBM Plex Mono"',
          'ui-monospace',
          'Consolas',
          'monospace',
        ],
      },
      colors: {
        ink: {
          50:  '#fbfff5',
          100: '#eff5e9',
          200: '#d6decf',
          300: '#aeb9a8',
          400: '#7d8778',
          500: '#697164',
          600: '#454d41',
          700: '#252a23',
          800: '#141713',
          900: '#070807',
        },
        accent: {
          DEFAULT: '#00ff22',
          50:  '#eaffed',
          100: '#c5ffce',
          200: '#88ff98',
          300: '#48ff5f',
          400: '#00ff22',
          500: '#00e61f',
          600: '#00b819',
          700: '#078519',
        },
        success: {
          DEFAULT: '#00ff22',
          50:  '#eaffed',
        },
        warn: {
          DEFAULT: '#d7ff3f',
        },
        danger: {
          DEFAULT: '#ff4d00',
        },
        panel: {
          DEFAULT: '#111311',
          soft: '#181c17',
          raised: '#1c1f1b',
        },
      },
      boxShadow: {
        'soft-sm': '0 0 0 1px rgba(255,255,255,0.06), 0 12px 34px rgba(0,0,0,0.32)',
        'soft':    '0 0 0 1px rgba(255,255,255,0.07), 0 22px 58px rgba(0,0,0,0.44)',
        'soft-md': '0 0 0 1px rgba(255,255,255,0.08), 0 24px 64px rgba(0,0,0,0.48)',
        'soft-lg': '0 0 0 1px rgba(0,255,34,0.12), 0 34px 96px rgba(0,0,0,0.58), 0 0 40px rgba(0,255,34,0.08)',
        'acid':    '0 0 0 1px rgba(0,255,34,0.28), 0 0 28px rgba(0,255,34,0.22)',
      },
      borderRadius: {
        xl2: '22px',
      },
      transitionTimingFunction: {
        'ios': 'cubic-bezier(.2,.9,.2,1)',
      },
      keyframes: {
        fadeIn: { from: { opacity: '0' }, to: { opacity: '1' } },
        rise:   { from: { transform: 'translateY(12px) scale(.985)', opacity: '0' }, to: { transform: 'translateY(0) scale(1)', opacity: '1' } },
        pulseDot: { '0%,100%': { opacity: '1' }, '50%': { opacity: '.35' } },
        borderSpin: { to: { transform: 'rotate(360deg)' } },
        marquee: { from: { transform: 'translateX(0)' }, to: { transform: 'translateX(-50%)' } },
        scan: { '0%': { transform: 'translateY(-100%)' }, '100%': { transform: 'translateY(100%)' } },
      },
      animation: {
        fadeIn: 'fadeIn .25s ease-ios both',
        rise:   'rise .28s ease-ios both',
        pulseDot: 'pulseDot 1.4s ease-in-out infinite',
        borderSpin: 'borderSpin 7s linear infinite',
        marquee: 'marquee 20s linear infinite',
        scan: 'scan 4.8s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
