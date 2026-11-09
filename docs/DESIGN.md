# claude-tower : direction visuelle

L'application doit ressembler à une application de bureau moderne (type Linear, Raycast, Warp), et non à un terminal. Le monospace est réservé au contenu des terminaux et aux chemins. Toute l'interface autour utilise la police du système.

## Principes

- **Interface en police système** : `system-ui, -apple-system, "Segoe UI", Cantarell, "Noto Sans", "DejaVu Sans", sans-serif`, 13 px de base. Monospace (`--mono`) uniquement pour les terminaux, les chemins et les raccourcis clavier.
- **Profondeur par couches** :
  - fond de la fenêtre ;
  - surfaces des cartes, surélevées ;
  - « écrans » des miniatures, plus sombres et enfoncés.
- Coins arrondis, bordures très fines et translucides, ombres douces.
- **Couleur d'état** : elle est portée par le cadre de la carte, la pastille d'état et la cible du prompt. Le reste de l'interface est neutre, et l'accent de marque est réservé aux actions principales.
- **Mouvement discret** : transitions de 120 à 180 ms sur le survol, la sélection et l'apparition. Une seule animation continue, la pulsation du halo des sessions qui attendent l'utilisateur, désactivée si `prefers-reduced-motion`.
- **Icônes** : SVG en ligne, trait de 1,5 px, 16 px, style Lucide, dessinées à la main dans un module `icons.ts` ou un composant `Icon.svelte`. Aucune dépendance ajoutée.

## Palette (sombre)

```css
--bg: #0b0e13;              /* fond de fenêtre */
--surface: #12161d;         /* carte */
--surface-2: #181d26;       /* éléments surélevés : barre d'app, compositeur, dialogues */
--screen: #07090c;          /* fond des miniatures et du plein écran */
--border: rgba(255,255,255,0.07);
--border-strong: rgba(255,255,255,0.14);
--text: #e7ecf3;
--text-2: #a7b0bd;          /* secondaire */
--text-3: #6c7684;          /* tertiaire, indices */
--accent: #d97757;          /* marque : bouton principal, focus */
--accent-hover: #e2896b;
--waiting: #ff5c5c;         /* attend l'utilisateur */
--busy: #f5c451;            /* travaille */
--background: #3fd0de;      /* fini, tâches de fond */
--idle: #7d8796;            /* fini */
--unknown: #7d8796;         /* inconnu (cadre pointillé) */
--danger: #ff5c5c;
--radius-lg: 12px; --radius-md: 8px; --radius-sm: 6px;
--shadow: 0 1px 0 rgba(255,255,255,0.03) inset, 0 8px 24px rgba(0,0,0,0.35);
```

Chaque couleur d'état existe aussi en version douce (`color-mix(in srgb, <couleur> 14%, transparent)`) pour les fonds de pastille et les halos.

## Structure de la fenêtre

```
┌──────────────────────────────────────────────────────────────────────┐
│ [logo] Claude Tower    ● 1 attend  ● 2 travaillent  ● 1 en fond  ○ 3 │  barre d'app (48 px)
│                                                [?]  [+ Nouvelle session] │
├──────────────────────────────────────────────────────────────────────┤
│ ┌───────────────────────────┐ ┌───────────────────────────┐          │
│ │ zest-fe         ● Attend  │ │ api       ● Travaille 4 min│          │  mosaïque (marge 12 px,
│ │ ~/zest · privée · 2 agents│ │ ~/api · tmux               │          │  espacement 10 px)
│ │ ┌───────────────────────┐ │ │ ┌───────────────────────┐ │          │
│ │ │  écran (miniature)    │ │ │ │                       │ │          │
│ │ └───────────────────────┘ │ │ └───────────────────────┘ │          │
│ │ ⚡ Bash: npm test          │ │ › Peux-tu corriger…       │          │  pied : dernière activité
│ └───────────────────────────┘ └───────────────────────────┘          │
├──────────────────────────────────────────────────────────────────────┤
│  ╭───────────────────────────────────────────────────────────────╮   │
│  │ [● zest-fe]  Message pour zest-fe…                       [➤]  │   │  compositeur
│  ╰───────────────────────────────────────────────────────────────╯   │
│   Entrée envoyer · Maj+Entrée nouvelle ligne · / pour écrire          │
└──────────────────────────────────────────────────────────────────────┘
```

### Barre d'application (48 px, `--surface-2`, bordure basse)

- **À gauche** : le logo (`frontend/src/assets/logo.svg`, une tour stylisée, 20 px), puis « Claude Tower » en 14 px semi-gras.
- **Au centre ou à gauche** : le résumé des états, sous forme de pastilles compactes. Chacune a un point coloré et un nombre (« 1 attend », « 2 travaillent », « 1 en fond », « 3 finies »). On masque une pastille à zéro. Si une session attend, sa pastille pulse doucement.
- **À droite** :
  - un bouton icône `?`, qui ouvre la fenêtre des raccourcis ;
  - le bouton principal « Nouvelle session » (`--accent`, icône +), qui fait comme la touche `n`.

### Carte de session

- **Conteneur** : `--surface`, `--radius-lg`, bordure `--border`, ombre `--shadow`, rembourrage de 10 px. Le corps s'organise en colonne : en-tête, écran, pied.
- **Cadre d'état** : une bordure de 1 px à la couleur d'état mélangée à 45 %. Un fin liseré de 2 px en haut de la carte, à la couleur d'état pleine, se lit d'un coup d'œil. Unknown : bordure en pointillé.
- **Sélection** : un anneau de 2 px à la couleur d'état (`box-shadow: 0 0 0 2px <état>`) plus un halo doux (`0 0 0 6px <état doux>`). Pas de changement de taille, transition de 150 ms.
- **Attente (waiting)** : le halo pulse (2,4 s, ease-in-out), que la carte soit sélectionnée ou non.
- **En-tête** :
  - ligne 1 : le nom (14 px, semi-gras, tronqué avec `…`), puis à droite la pastille d'état. La pastille est une capsule au fond de couleur douce, avec un point et le libellé (« Attend une autorisation », « Travaille », « Fini · tâches de fond », « Fini »). La durée suit en `--text-3`. Pendant le travail, le point a une petite animation (pulsation ou anneau qui tourne) ;
  - ligne 2 : une icône de dossier et le dossier en mono 11,5 px `--text-2`, puis des étiquettes discrètes : `privée` / `tmux` / `hors tmux`, et le badge des tâches de fond s'il existe (« 2 agents · 1 shell »).
  - Au survol de la carte, des boutons icônes apparaissent en haut à droite, à la place de la durée : Plein écran (`o`), Aller au terminal (`Entrée`), et Arrêter (`x`) seulement pour une session privée. Chacun a une infobulle (`title`) qui donne son raccourci.
- **Écran** : un conteneur `--screen` avec `--radius-md` et une bordure intérieure `--border`, qui occupe tout l'espace restant. La miniature xterm y est placée en haut à gauche. L'espace restant reste au fond `--screen`, comme un vrai écran. Le marqueur « écran indisponible » est un voile centré, avec une icône.
- **Pied** : une ligne, avec l'icône de l'activité (outil : éclair ou terminal ; texte de Claude : bulle ; prompt de l'utilisateur : flèche), puis la dernière activité en 12 px `--text-2`, tronquée. Hors tmux, le corps montre l'activité en grand texte lisible (police système, pas de bloc `pre` brut), avec une note « Lecture seule : session hors tmux ».

### Compositeur (bas de fenêtre)

- Une barre flottante, avec une marge de 12 px autour, sur fond `--surface-2`, en `--radius-lg`, avec une bordure et une ombre.
- **À gauche**, la puce de cible : un point d'état et le nom de la session sélectionnée. Elle est cliquable et fait défiler jusqu'à la carte.
- **Au centre**, la zone de texte s'agrandit avec le contenu, de 1 à 8 lignes. L'indice de saisie est « Message pour <nom>… ».
- **À droite**, le bouton d'envoi : une icône d'avion en papier, `--accent`, désactivé si le texte est vide.
- **Désactivé** (`CanPrompt` faux) : une icône de cadenas, la raison en `--text-2` à la place de l'indice, et le champ grisé.
- **Pendant l'envoi** : une petite animation de chargement dans le bouton.
- **Au focus** : un anneau `--accent`.
- **Sous la barre** : une ligne d'aide en 11 px `--text-3`. Les raccourcis s'affichent comme des touches de clavier (`<kbd>` stylé : fond `--surface`, bordure, rayon 4 px, mono 11 px).

### Plein écran

- **Barre du haut** (48 px, `--surface-2`) : le bouton « ← Retour à la grille » avec son raccourci (`<kbd>Ctrl</kbd><kbd>Maj</kbd><kbd>Q</kbd>`), puis le nom de la session, la pastille d'état et le dossier.
- Le terminal occupe le reste, sur fond `--screen`, avec 8 px de marge intérieure.
- L'ouverture se fait en fondu et léger agrandissement (150 ms).

### Dialogues, notifications et raccourcis

- **Dialogue** (confirmation d'arrêt) :
  - voile `rgba(0,0,0,0.55)` avec `backdrop-filter: blur(4px)`, panneau `--surface-2` de 420 px, `--radius-lg` ;
  - titre en 15 px semi-gras, texte en `--text-2` ;
  - bouton « Annuler » secondaire, qui a le focus par défaut, et bouton « Arrêter la session » en danger.
- **Notifications (toasts)** : elles remplacent les bandeaux.
  - pile en bas à droite, au-dessus du compositeur ;
  - chacune a une icône, un texte, une fermeture, et disparaît seule après 5 s (les erreurs après 8 s) ;
  - genres : info, avertissement, erreur ;
  - `StartupWarning` y apparaît une fois.
- **Fenêtre des raccourcis** (touche `?` ou bouton) : un dialogue qui liste toutes les touches, groupées en Navigation, Session et Compositeur.

### États vides et d'erreur

- **Aucune session** : une illustration SVG simple et centrée (la tour, en grand et en traits fins), le titre « Aucune session Claude en cours », un texte « Lancez une session ici, ou utilisez `claude-tower run` dans un terminal. », et le bouton principal « Nouvelle session ».
- **Page d'erreur** (tmux absent ou trop ancien) : la même mise en page soignée, avec une icône d'alerte et la commande d'installation dans un bloc de code copiable (bouton « Copier »).

## Logo

Une tour stylisée, géométrique et simple : un rectangle étroit couronné de trois créneaux, avec une petite fenêtre lumineuse, ou une antenne qui émet trois arcs. Tracé en `--accent` sur un fond transparent. Il est utilisé :
- dans la barre d'application ;
- dans l'état vide ;
- comme icône de l'application : `build/appicon.png` en 1024 px, généré à partir du SVG, sur un fond `#12161d` aux coins arrondis.

## Accessibilité

- Contrastes d'au moins 4,5:1 pour le texte principal.
- Focus visible au clavier sur tout élément interactif (anneau `--accent`).
- Libellés `aria-label` sur les boutons icônes.
