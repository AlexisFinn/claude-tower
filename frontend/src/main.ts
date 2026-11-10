import { mount } from 'svelte'
import './style.css'
import './assets/fonts/fonts.css'
import '@xterm/xterm/css/xterm.css'
import { loadThumbnailFont } from './lib/thumbnail'

// The cards measure the thumbnail font once, when their module loads, so the
// application is imported only after the font is loaded; a slow or failed
// load does not hold it back for long.
await Promise.race([loadThumbnailFont(), new Promise((resolve) => setTimeout(resolve, 2000))])
const { default: App } = await import('./App.svelte')

mount(App, { target: document.getElementById('app')! })
