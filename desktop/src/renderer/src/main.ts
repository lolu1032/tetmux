import '@xterm/xterm/css/xterm.css'
import './style.css'
import { App } from './app'

const root = document.getElementById('app')
if (!root) throw new Error('#app root element missing')

void new App(root).start()
