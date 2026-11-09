/** One shortcut: the alternative key combinations that trigger it, and what it does. */
export interface Shortcut {
  combos: string[][]
  label: string
}

/** A titled group of shortcuts. */
export interface ShortcutGroup {
  title: string
  items: Shortcut[]
}

/** Every keyboard shortcut of the application, by group. */
export const SHORTCUT_GROUPS: ShortcutGroup[] = [
  {
    title: 'Navigation',
    items: [
      { combos: [['h'], ['←']], label: 'Sélectionner la carte de gauche' },
      { combos: [['l'], ['→']], label: 'Sélectionner la carte de droite' },
      { combos: [['k'], ['↑']], label: 'Sélectionner la carte du dessus' },
      { combos: [['j'], ['↓']], label: 'Sélectionner la carte du dessous' },
      { combos: [['?']], label: 'Afficher cette aide' },
    ],
  },
  {
    title: 'Session',
    items: [
      { combos: [['Entrée']], label: 'Aller au terminal de la session' },
      { combos: [['o']], label: 'Ouvrir en plein écran' },
      { combos: [['n']], label: 'Nouvelle session' },
      { combos: [['x']], label: 'Arrêter la session (lancée par l’application)' },
      { combos: [['Ctrl', 'Maj', 'Q']], label: 'Quitter le plein écran' },
      { combos: [['Ctrl', 'Maj', 'C']], label: 'Copier la sélection (plein écran)' },
      { combos: [['Ctrl', 'Maj', 'V']], label: 'Coller (plein écran)' },
      { combos: [['Maj', 'Entrée']], label: 'Nouvelle ligne (plein écran)' },
    ],
  },
  {
    title: 'Compositeur',
    items: [
      { combos: [['i'], ['/']], label: 'Écrire à la session sélectionnée' },
      { combos: [['Entrée']], label: 'Envoyer le message' },
      { combos: [['Maj', 'Entrée']], label: 'Nouvelle ligne' },
      { combos: [['Échap']], label: 'Retour à la grille' },
    ],
  },
]
