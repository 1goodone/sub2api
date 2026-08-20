/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // 主色调 - clean SaaS blue
        primary: {
          50: '#eff7ff',
          100: '#dbeeff',
          200: '#b8ddff',
          300: '#84c5ff',
          400: '#48a5ff',
          500: '#1687f8',
          600: '#0872df',
          700: '#075bb5',
          800: '#0b4d91',
          900: '#103f72',
          950: '#0b284b'
        },
        // 辅助色 - 深蓝灰
        accent: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617'
        },
        // 深色模式背景
        dark: {
          50: '#f8fafc',
          100: '#f1f5f9',
          200: '#e2e8f0',
          300: '#cbd5e1',
          400: '#94a3b8',
          500: '#64748b',
          600: '#475569',
          700: '#334155',
          800: '#1e293b',
          900: '#0f172a',
          950: '#020617'
        }
      },
      fontFamily: {
        sans: [
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
        glass: '0 2px 6px -2px rgba(15, 23, 42, 0.08), 0 26px 60px -26px rgba(15, 23, 42, 0.3)',
        'glass-sm': '0 2px 5px -2px rgba(15, 23, 42, 0.07), 0 16px 34px -18px rgba(15, 23, 42, 0.24)',
        glow: '0 8px 24px rgba(22, 135, 248, 0.22)',
        'glow-lg': '0 16px 40px rgba(22, 135, 248, 0.24)',
        card: '0 2px 5px -2px rgba(15, 23, 42, 0.08), 0 14px 32px -18px rgba(15, 23, 42, 0.24)',
        'card-hover': '0 4px 10px -4px rgba(15, 23, 42, 0.11), 0 22px 46px -18px rgba(15, 23, 42, 0.32)',
        popup: '0 10px 24px -8px rgba(15, 23, 42, 0.24), 0 28px 56px -22px rgba(15, 23, 42, 0.34)',
        modal: '0 28px 88px -24px rgba(15, 23, 42, 0.48), 0 10px 28px -10px rgba(15, 23, 42, 0.24)',
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
        lg: '0.75rem',
        xl: '1rem',
        '2xl': '1.25rem',
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}
