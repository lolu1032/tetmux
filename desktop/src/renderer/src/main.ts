import '@xterm/xterm/css/xterm.css'
import './style.css'
import { App } from './app'

const root = document.getElementById('app')
if (!root) throw new Error('#app root element missing')

// Expose the OS to CSS (e.g. macOS indents the sidebar brand past the inset
// traffic-light buttons). Falls back to '' if the preload bridge is absent.
document.body.dataset.platform = window.tetmux?.platform ?? ''

void new App(root).start()
