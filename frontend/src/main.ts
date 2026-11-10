import { mount } from 'svelte'
import './style.css'
import './assets/fonts/fonts.css'
import '@xterm/xterm/css/xterm.css'
import App from './App.svelte'
import { loadThumbnailFont } from './lib/thumbnail'

// The cards measure the thumbnail font once, so it is loaded before they mount;
// a slow load does not hold the application back for long.
await Promise.race([loadThumbnailFont(), new Promise((resolve) => setTimeout(resolve, 2000))])

const app = mount(App, { target: document.getElementById('app')! })

export default app
