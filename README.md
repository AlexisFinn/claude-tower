# Claude Tower

Une application de bureau (Linux et macOS) pour **voir et piloter d'un seul écran toutes les sessions Claude Code** qui tournent sur ta machine.

- **Mosaïque** : une carte par session, qui s'adapte au nombre de sessions et occupe toute la fenêtre. Le cadre de chaque carte prend la couleur de l'état de la session. La carte montre l'écran de la session en direct, panneau des tâches compris.
- **Compositeur** : la barre du bas envoie un message à la session sélectionnée, sans quitter l'application.
- **Aller à la session** : l'application bascule vers le terminal où la session est déjà affichée, ou l'ouvre en plein écran dans l'application.
- **Nouvelle session** : lance Claude dans le dossier de ton choix. La session tourne dans un serveur tmux privé et continue même si tu fermes l'application.

Tout est local : pas de serveur, pas de compte, pas de réseau. L'application lit le registre des sessions de Claude Code (`~/.claude/sessions/`), la fin des transcripts, et les serveurs tmux de ta session utilisateur.

## États

| Couleur | Signification |
| --- | --- |
| 🔴 rouge, halo pulsé | la session **attend ton intervention** (autorisation, question, dialogue) |
| 🟡 jaune | la session travaille (tour en cours ou commande shell) |
| 🔵 cyan | le tour est fini, mais des agents ou des shells tournent encore en arrière-plan |
| ⚪ gris | la session a fini et attend un nouveau message |
| gris pointillé | état non reconnu |

## Installation

Prérequis :
- **tmux 3.2 ou plus** (`dnf install tmux`, `apt install tmux`, `brew install tmux`) ;
- **Claude Code** ;
- pour construire l'application : Go 1.26, Node 22, et le CLI Wails v2.17.1 (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`).

Bibliothèques système pour construire :
- **Fedora** : `sudo dnf install gtk3-devel webkit2gtk4.1-devel` ;
- **Debian ou Ubuntu** : `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev` ;
- **macOS** : les outils en ligne de commande de Xcode (`xcode-select --install`).

```sh
cd frontend && npm install && cd ..
make install
```

- **Linux** : `make install` place le binaire dans `~/.local/bin/claude-tower` et l'icône dans `~/.local/share/pixmaps/`. Il ajoute aussi un lanceur, « Claude Tower », dans le menu des applications. `make uninstall` retire le tout.
- **macOS** : `make install` copie `claude-tower.app` dans `/Applications`, et crée le lien `~/.local/bin/claude-tower`. L'application n'est pas signée : au premier lancement, fais clic droit puis « Ouvrir », ou lance `xattr -dr com.apple.quarantine /Applications/claude-tower.app`.

`wails doctor` peut signaler à tort que « libwebkit » manque sur les systèmes qui n'ont que WebKitGTK 4.1 (Fedora 40 et plus). La construction passe quand même, avec le tag `webkit2_41` que le `Makefile` ajoute.

## Utilisation

### Sessions visibles

| Session | Dans la carte | Compositeur | « Aller à la session » |
| --- | --- | --- | --- |
| lancée depuis l'application, ou avec `claude-tower run` | écran en direct | oui | plein écran intégré, ou terminal où elle est affichée |
| lancée dans ton propre tmux | écran en direct | oui | bascule ton terminal tmux et le met au premier plan (sous Wayland : plein écran intégré) |
| lancée hors tmux | dernière activité, en lecture seule | non | met la fenêtre du terminal au premier plan si possible |

### Sans tmux dans tes terminaux : `claude-tower run`

Si tu lances d'habitude `claude` directement, remplace la commande par `claude-tower run`. Elle accepte les mêmes arguments, transmis tels quels : `claude-tower run --resume`, `claude-tower run --model opus`…

Tu vois Claude dans ton terminal comme d'habitude, mais la session tourne dans le serveur privé de l'application, qui peut donc l'afficher et la piloter. Un alias permet de ne rien changer à tes habitudes :

```sh
alias claude='claude-tower run'
```

Fermer la fenêtre du terminal détache seulement la session : elle continue, et tu la retrouves dans l'application.

Dans ce terminal, la souris sert à défiler. Pour sélectionner du texte, maintiens `Maj` enfoncé.

### Raccourcis

| Touche | Action |
| --- | --- |
| `←` `↓` `↑` `→` ou `h` `j` `k` `l` | changer de carte |
| clic | sélectionner une carte |
| `Entrée` ou double-clic | aller à la session |
| `o` | ouvrir la session en plein écran dans l'application |
| `i` ou `/` | écrire dans le compositeur |
| `Entrée` (compositeur) | envoyer |
| `Maj+Entrée` (compositeur) | nouvelle ligne |
| `Échap` (compositeur) | revenir à la grille |
| `n` | nouvelle session |
| `x` | arrêter la session (seulement une session lancée par l'application) |
| `?` | liste des raccourcis |
| `Ctrl+Maj+Q` (plein écran) | revenir à la grille |
| `Ctrl+Maj+C` et `Ctrl+Maj+V` (plein écran) | copier et coller |

Le compositeur est désactivé quand la session attend une autorisation ou une réponse, pour éviter que ton message valide un choix par défaut. Réponds alors depuis le plein écran.

### En ligne de commande

- `claude-tower list` : liste des sessions en texte brut.
- `claude-tower bar` : segment pour la barre d'état tmux, par exemple `set -g status-right '#(~/.local/bin/claude-tower bar) '`.

### Configuration

`~/.config/claude-tower/config.json` (sous macOS : `~/Library/Application Support/claude-tower/config.json`) :

```json
{
  "claudeCommand": ["claude"],
  "tmuxTmpDir": "",
  "claudeConfigDir": ""
}
```

- `claudeCommand` : la commande qui lance Claude, avec ses arguments éventuels, par exemple `["claude", "--permission-mode", "auto"]`. Les alias du shell ne sont pas vus par l'application.
- `tmuxTmpDir` : à renseigner si tu définis `TMUX_TMPDIR` dans ton shell et que tu lances l'application depuis le menu du bureau.
- `claudeConfigDir` : le dossier de configuration de Claude Code, si ce n'est pas `~/.claude`.

## Limites

- **Format interne** : le registre des sessions est interne à Claude Code et peut changer à une mise à jour. Tout ce qui en dépend est dans `internal/registry`.
- **Wayland** : l'application ne peut pas mettre au premier plan la fenêtre d'une autre application. « Aller à la session » ouvre alors le plein écran intégré.
- **gnome-terminal** : comme toutes ses fenêtres appartiennent au même processus, c'est la première fenêtre trouvée qui passe au premier plan.
- **Secrets visibles** : les cartes affichent l'écran des sessions, commandes comprises. Ce qui y est écrit est visible.
- **Brouillon** : un brouillon déjà tapé dans le prompt de Claude part avec le message envoyé depuis le compositeur.
- **Sessions `run`** : elles survivent à leur terminal. Arrête-les depuis l'application (`x`).
- **macOS** : un PID réutilisé peut faire réapparaître brièvement une session morte.

## Checklist macOS (avant de partager)

1. `make install` réussit et l'application s'ouvre.
2. Les sessions Claude en cours apparaissent, et l'état « attend » s'affiche lors d'une demande d'autorisation.
3. `n` lance une session. Elle survit à la fermeture de l'application, et `x` l'arrête.
4. `claude-tower run` dans Terminal.app ou iTerm2 : la session apparaît, avec son écran en direct.
5. `Entrée` sur une session tmux met le terminal au premier plan (accepter les autorisations « Automatisation » et « Accessibilité » demandées).
6. Plein écran : frappe, `Maj+Entrée`, copier-coller, retour.
7. `tmux -V` vaut 3.2 ou plus, et `infocmp tmux-256color` ne renvoie pas d'erreur (sinon l'application utilise `screen-256color`).

## Développement

```sh
make test     # vet et tests Go, tests et typage du frontend
make build    # build/bin/claude-tower
wails dev -tags webkit2_41   # interface rechargée à chaud
```

- Le plan d'implémentation est dans `PLAN.md`, et la direction visuelle dans `docs/DESIGN.md`.
- Les tests d'intégration démarrent de vrais serveurs tmux sur des sockets temporaires. Ils ne touchent jamais à tes sessions.
