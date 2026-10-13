/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // Shared blue accent, with separate readable light/dark control colors.
        primary: {
          50: '#eff4ff',
          100: '#e0eaff',
          200: '#c5d7ff',
          300: '#a2bdff',
          400: '#7b9ff5',
          500: '#4f7ee3',
          600: '#3264d8',
          700: '#2852b6',
          800: '#264790',
          900: '#253e72',
          950: '#172747'
        },
        // Neutral surfaces keep semantic and brand colors distinct.
        accent: {
          50: '#fafafa',
          100: '#f4f4f5',
          200: '#e4e4e7',
          300: '#d4d4d8',
          400: '#a1a1aa',
          500: '#71717a',
          600: '#52525b',
          700: '#3f3f46',
          800: '#27272a',
          900: '#18181b',
          950: '#101113'
        },
        // 深色模式背景
        dark: {
          50: '#fafafa',
          100: '#f4f4f5',
          200: '#e4e4e7',
          300: '#d4d4d8',
          400: '#a1a1aa',
          500: '#80808b',
          600: '#55555f',
          700: '#36383f',
          800: '#24262c',
          900: '#191b20',
          950: '#101113'
        }
      },
      fontFamily: {
        sans: [
          'Noto Sans SC Variable',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Segoe UI',
          'Roboto',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: '0 1px 3px rgba(24, 24, 27, 0.04)',
        'glass-sm': '0 1px 2px rgba(24, 24, 27, 0.03)',
        glow: '0 8px 24px rgba(22, 135, 248, 0.22)',
        'glow-lg': '0 16px 40px rgba(22, 135, 248, 0.24)',
        card: '0 1px 2px rgba(24, 24, 27, 0.03)',
        'card-hover': '0 3px 10px rgba(24, 24, 27, 0.06)',
        popup: '0 4px 12px rgba(24, 24, 27, 0.06), 0 18px 48px -12px rgba(24, 24, 27, 0.18)',
        modal: '0 24px 80px -16px rgba(0, 0, 0, 0.32)',
        'inner-glow': 'inset 0 1px 0 rgba(255, 255, 255, 0.1)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #1687f8 0%, #0872df 100%)',
        'gradient-dark': 'linear-gradient(135deg, #1e293b 0%, #0f172a 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.1) 0%, rgba(255,255,255,0.05) 100%)',
        'mesh-gradient':
          'linear-gradient(rgba(22,135,248,0.025) 1px, transparent 1px), linear-gradient(90deg, rgba(22,135,248,0.025) 1px, transparent 1px)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 8px 20px rgba(22, 135, 248, 0.18)' },
          '100%': { boxShadow: '0 12px 28px rgba(22, 135, 248, 0.26)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        lg: '0.5rem',
        xl: '0.75rem',
        '2xl': '1rem',
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
