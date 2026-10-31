import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import QuarantineWindow from './screens/QuarantineWindow'
import './styles.css'

// The set-aside window is this bundle at ?view=quarantine.
const view = new URLSearchParams(window.location.search).get('view')

createRoot(document.getElementById('root')!).render(
  <StrictMode>{view === 'quarantine' ? <QuarantineWindow /> : <App />}</StrictMode>,
)
