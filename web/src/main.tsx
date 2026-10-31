import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import { App } from './App'
import { ThemeProvider } from './lib/theme/useTheme'
import { ToastProvider } from './lib/toast'

// The dashboard always follows the system theme (public/theme.js). EasyUI's theme helper
// would otherwise remember a past choice in storage and override the system.
try {
  localStorage.removeItem('easyui-theme')
} catch {
  /* storage may be blocked */
}
const systemTheme = matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider initialTheme={systemTheme}>
      <ToastProvider>
        <App />
      </ToastProvider>
    </ThemeProvider>
  </StrictMode>,
)
