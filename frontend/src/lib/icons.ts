const CIRCLE = 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Z'

/** Path data of the icons, drawn on a 24 x 24 grid and stroked. */
export const ICONS = {
  plus: ['M12 5v14', 'M5 12h14'],
  file: ['M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9Z', 'M14 3v6h6'],
  help: [CIRCLE, 'M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3', 'M12 17h.01'],
  folder: [
    'M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.7-.9l-.8-1.2A2 2 0 0 0 7.9 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z',
  ],
  maximize: ['M8 3H5a2 2 0 0 0-2 2v3', 'M21 8V5a2 2 0 0 0-2-2h-3', 'M3 16v3a2 2 0 0 0 2 2h3', 'M16 21h3a2 2 0 0 0 2-2v-3'],
  terminal: ['m4 17 6-6-6-6', 'M12 19h8'],
  power: ['M12 2v10', 'M18.4 6.6a9 9 0 1 1-12.8 0'],
  zap: ['M13 2 3 14h9l-1 8 10-12h-9l1-8Z'],
  message: ['M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2Z'],
  arrowRight: ['M5 12h14', 'm12 5 7 7-7 7'],
  arrowLeft: ['M19 12H5', 'm12 19-7-7 7-7'],
  send: ['m22 2-7 20-4-9-9-4Z', 'M22 2 11 13'],
  lock: ['M5 11h14a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2Z', 'M7 11V7a5 5 0 0 1 10 0v4'],
  x: ['M18 6 6 18', 'm6 6 12 12'],
  info: [CIRCLE, 'M12 16v-4', 'M12 8h.01'],
  warning: ['m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z', 'M12 9v4', 'M12 17h.01'],
  error: [CIRCLE, 'M12 8v4', 'M12 16h.01'],
  copy: [
    'M10 8h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2Z',
    'M4 16a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h10a2 2 0 0 1 2 2',
  ],
  check: ['M20 6 9 17l-5-5'],
  monitorOff: ['M3 4h18a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1Z', 'M8 21h8', 'M12 17v4', 'm3 3 18 18'],
  loader: ['M21 12a9 9 0 1 1-6.2-8.6'],
} as const

/** The name of an icon. */
export type IconName = keyof typeof ICONS
