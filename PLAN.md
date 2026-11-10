# claude-tower (bureau) : plan d'implémentation

Application de bureau pour Linux et macOS, à partager avec l'équipe. Elle affiche toutes les sessions Claude Code de la machine sous forme de cartes, et permet de les piloter sans quitter la fenêtre :
- une grille de cartes à l'ordre fixe, avec un cadre coloré selon l'état et le bas de l'écran de la session en direct ;
- sélection aux flèches, aux touches `hjkl` ou au clic ;
- `Entrée` ou double-clic va à la session : vers le terminal où elle est déjà affichée, sinon dans un terminal plein écran intégré ;
- une barre de prompt en bas, qui envoie le message à la session sélectionnée ;
- une touche pour lancer une nouvelle session, une autre pour en arrêter une.

La version terminal (branche `main`, jusqu'au commit `2943b3a`) sert de base :
- `internal/registry`, `internal/transcript` et `internal/view` (états, libellés, durées) ;
- la commande `bar`, qui est conservée.

`internal/tui` et `internal/tmuxctl` sont supprimés. La remontée des PID parents part dans `internal/locate`.

## Décisions

- **Technologie** : Wails v2 (Go, avec la vue web du système), frontend Svelte + TypeScript, terminaux xterm.js.
- **tmux 3.2 ou plus est obligatoire.** Il est vérifié au démarrage, avec un message explicite.
- **Sessions lancées depuis l'application** : dans un serveur tmux privé et caché. Elles survivent à la fermeture de l'application.
- **`claude-tower run [args…]`** remplace `claude` pour ceux qui n'utilisent pas tmux. Les arguments sont transmis tels quels à claude. La session tourne dans le serveur privé, et `run` s'y rattache dans le terminal courant. Fermer ce terminal **détache seulement** : la session continue.
- **Arrêter une session** : touche `x`, avec confirmation qui montre le nom et le PID. Seulement pour les sessions du serveur privé.
- **Les trois cas de session** :

  | Session | Corps de la carte | Prompt | « Aller à la session » |
  | --- | --- | --- | --- |
  | Dans tmux, affichée par au moins un client | bas de l'écran réel | oui | X11 et macOS : bascule ce client et met son terminal au premier plan. Wayland ou échec : plein écran intégré |
  | Dans tmux, aucun client | bas de l'écran réel | oui | plein écran intégré |
  | Hors tmux | résumé du transcript | non | met au premier plan la fenêtre du terminal (X11, macOS), sinon message |

- **Prompt, en liste blanche** : l'envoi n'est autorisé que si la session est dans tmux et que son statut est `idle`, `busy` ou `shell`. Tout `waiting` et tout statut inconnu le bloquent, avec la raison affichée ; on répond alors depuis le plein écran. L'état est relu **juste avant le collage**.
- **Ordre des cartes fixe** : par date de début de la session (`StartedAt`), puis par PID. L'urgence se voit à la couleur du cadre : Waiting rouge, Busy jaune, Background cyan, Idle gris, Unknown gris pointillé. La carte sélectionnée a un cadre plus épais.
- **Corps de la carte** : les dernières lignes de l'écran, à une taille lisible (police de 11 px), coupées à droite. Le plein écran montre tout.
- **macOS** : il n'est pas testable sur le poste de développement. Un collègue déroule la checklist macOS du README avant le partage.
- **Pas de Windows natif.**

## Faits établis (vérifiés le 2026-11-09)

### Registre et états

Voir le code existant et l'ancien plan (`git show main:PLAN.md`).
- `status` : `busy`, `shell`, `idle`, `waiting`.
- `waitingFor` : liste ouverte (`permission prompt`, `dialog open`, `input needed`…).
- `claude -p` (`entrypoint: sdk-cli`) est filtré.
- Le champ `tmux` du registre n'a pas la socket : on localise les sessions par la recherche du lot A.
- **Linux seulement** : `registry.procAlive` lit `/proc/<pid>/stat`. Sous macOS, il faut un autre test de vivacité.

### tmux (3.8 sur le poste ; certaines commandes demandent 3.2 ou plus)

**Sockets et serveurs**
- Les sockets de l'utilisateur sont dans `${TMUX_TMPDIR:-/tmp}/tmux-<uid>/`. Une socket peut être orpheline : la commande échoue, on l'ignore.
- `-f` n'est lu qu'au démarrage du serveur, et un fichier absent est ignoré sans erreur. Sur un serveur vivant, on recharge avec `source-file`.
- Un serveur garde **son** environnement : `new-session` ne transmet au processus que ce qui est passé explicitement.

**Limites des commandes**
- Une commande tmux de plus d'environ 16 Kio échoue (« command too long ») : c'est le cas de `set-buffer` et de `-e` répétés.
- Les arguments d'une commande sont lisibles dans `/proc/<pid>/cmdline`.

**Volets et sessions groupées**
- `list-panes -a` liste un volet une fois par session du groupe.
- Le volet actif est propre à la fenêtre, donc commun à tout le groupe : `select-pane` dans une session de vue change le volet actif de l'utilisateur.
- `set-option destroy-unattached on` sur une session **jamais attachée** la détruit aussitôt.
- `tmux attach -t <vue> \; set-option -t <vue> destroy-unattached on` fonctionne : la session est détruite quand son client disparaît.
- Cibles : les identifiants `$N` (session), `@N` (fenêtre) et `%N` (volet) marchent avec toutes les commandes. `=nom` ne marche pas avec `set-option` (il faut `=nom:`). **On cible donc toujours par identifiant.**

**Collage d'un prompt**
- `paste-buffer -p` (collage entre crochets) envoie `\e[200~…\e[201~`, mais seulement si l'application l'a demandé, ce que fait Claude Code. Sans `-S`, il neutralise les ESC (`^[`) et les autres contrôles. LF devient CR.
- Un `send-keys Enter` qui suit arrive après `\e[201~`.
- Si le volet est en copy-mode, l'Entrée est consommée par le mode. On l'annule de façon atomique : `if-shell -F -t <volet> '#{pane_in_mode}' 'send-keys -t <volet> -X cancel'`.
- `load-buffer -b <nom> -` lit le texte sur l'entrée standard, sans limite de taille.

**Divers**
- `capture-pane -p -e` renvoie l'écran visible : lignes séparées par `\n` seul, avec un `\n` final.
- Une session détachée sans `-x`/`-y` fait 80×24.
- `tmux attach` sans `TERM` échoue. Sans locale UTF-8, les caractères non ASCII deviennent `_`.

### Environnement

**Lanceur de bureau ou Finder**
- `PATH` réduit. `TERM`, `LANG` et `TMUX_TMPDIR` sont souvent absents. Les variables du shell (`.zshrc`) manquent.
- `$SHELL -lc` ne lit pas `.zshrc`. `$SHELL -ilc 'command -v claude'` peut renvoyer un alias ou une fonction, et non un chemin.

**Poste de développement**
- Go 1.26, Node 22, npm, X11.
- Wails v2.17.1, `creack/pty` v1.1.24 et `jezek/xgb` v1.3.1 sont disponibles.
- Fedora : `gtk3-devel`, `webkit2gtk4.1-devel`, et le tag `webkit2_41`.

**Compilation**
- Le paquet racine (Wails, cgo, `//go:embed all:frontend/dist`) ne compile qu'avec les dépendances système, le tag, et un `frontend/dist` non vide.
- Tout ce qui est testable vit donc dans `internal/`, sans import de Wails.

**Dépendances Go** : `go get <module>/...@<version>` remplit `go.sum` pour tous les paquets. `go mod tidy` ne se lance qu'en ligne, par l'utilisateur.

## Arborescence cible

```
claude-tower/
├── main.go                    # Wails bootstrap; delegates subcommands to internal/cli
├── wails.json
├── frontend/                  # Svelte + TS; frontend/dist/.gitkeep committed; wailsjs/ committed
├── internal/cli/              # run, list, bar, exec-env (no Wails import)
├── internal/registry/         # existing; portable liveness
├── internal/transcript/       # existing
├── internal/view/             # existing
├── internal/tmux/             # binary and version, servers, panes, clients, capture, paste, view sessions
├── internal/locate/           # process tree, session→pane matching
├── internal/hosting/          # private server: socket, config, environment, new session, run, kill
├── internal/focus/            # raise a terminal window: X11, macOS, otherwise ErrUnsupported
├── internal/core/             # snapshot, cards, screen, prompt, focus, new/kill session
├── internal/ptyterm/          # embedded full-screen terminal
└── internal/app/              # methods bound to Wails; Wails calls behind interfaces
```

## Lots

### Lot 0 (utilisateur) : outillage et squelette

Commandes données par l'orchestrateur, lancées par l'utilisateur :

```sh
sudo dnf install gtk3-devel webkit2gtk4.1-devel
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1
cd ~/Projects/claude-tower
go get github.com/jezek/xgb/...@v1.3.1 github.com/creack/pty/...@v1.1.24 github.com/wailsapp/wails/v2/...@v2.17.1
wails init -n claude-tower -t svelte-ts -d ~/Projects/claude-tower-skeleton
cp -r ~/Projects/claude-tower-skeleton/frontend ~/Projects/claude-tower-skeleton/wails.json .
rm -rf ~/Projects/claude-tower-skeleton
cd frontend && npm install && npm install @xterm/xterm @xterm/addon-fit @xterm/addon-unicode11 @xterm/addon-clipboard && npm install -D vitest svelte-check
cd .. && wails doctor
```

- L'orchestrateur relit ensuite le squelette copié et ajoute `frontend/dist/.gitkeep`.
- `main.go` et `app.go` du squelette ne sont pas repris : le lot B écrit les siens.
- Après le lot B, l'utilisateur lance `go mod tidy`.

### Lot A (Sonnet, effort high) : moteur, en deux sous-lots

- **A1** : `registry` (vivacité portable), `tmux`, `locate`, et `cli` (migration de `list` et `bar`, `main.go` minimal).
- **A2**, après revue et commit de A1 : `hosting`, `focus`, `core`, et `run` / `exec-env` dans `cli`.

Uniquement du Go, dans `internal/`, sans Wails.
- Tout appel externe passe par un `Runner` injectable avec délai maximal (`exec.CommandContext`, 2 s par défaut).
- L'erreur d'un `Runner` inclut le stderr de la commande.

#### `internal/registry` (modification)

- La vivacité est portable.
  - Linux : inchangée (`/proc` et `procStart`).
  - Ailleurs : `syscall.Kill(pid, 0)`, sans vérifier `procStart`.
- Le comportement est choisi par `runtime.GOOS` au moment de construire le `Reader` par défaut, et reste injectable.
- Test avec `Alive` injecté, et test du choix par `GOOS` (fonction pure).

#### `internal/tmux`

```go
type Runner func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

type Server struct{ Socket, Bin string; Run Runner } // every command uses -S Socket

type Pane struct {
    Server        Server
    ID            string // "%N"
    PID           int    // pane_pid
    SessionID     string // "$N"
    SessionName   string
    WindowID      string // "@N"
    Active        bool   // pane_active
    SessionGroup  string // session_group, "" when ungrouped
    Width, Height int
}

type Client struct{ Name string; PID int; SessionID, SessionGroup string; Activity int64 }

func FindBinary(lookPath func(string) (string, error)) (string, error) // PATH, then /opt/homebrew/bin, /usr/local/bin, /usr/bin
func CheckVersion(ctx, bin string, run Runner) error                   // tmux -V >= 3.2, explicit message otherwise
func Servers(dir string, bin string, run Runner) []Server             // sockets of dir; candidates only, failures are handled by callers
func (s Server) Panes(ctx) ([]Pane, error)
func (s Server) Clients(ctx) ([]Client, error)
func (s Server) Capture(ctx, pane string) (string, error)
func (s Server) Paste(ctx, pane, text string) error
func (s Server) SwitchClient(ctx, client, pane string) error
func (s Server) NewViewSession(ctx, pane Pane) (ViewSession, error)
func (s Server) RestoreActivePane(ctx, v ViewSession) error
func (s Server) KillSession(ctx, sessionID string) error
func (s Server) SweepViewSessions(ctx) error
```

**Lecture des volets et clients**
- Formats `-F` séparés par `\t`.
- `Panes` ignore les sessions préfixées `claude-tower-view-` et **déduplique par `pane_id`**.
- `Clients` ignore les clients des sessions de vue.
- Une session est « affichée » (`Attached`) si un client est attaché à la même session ou à une session du même groupe (`#{session_group}`), quelle que soit la fenêtre courante de ce client.

**Collage : `Paste`**
0. `Sanitize(text)` (fonction pure) : CRLF et CR seuls deviennent LF, puis tous les contrôles C0 et C1 sont retirés sauf `\n` et `\t` (ESC compris). Le collage ne dépend ainsi pas de l'assainissement propre à chaque version de tmux ;
1. `if-shell -F -t <pane> '#{pane_in_mode}' 'send-keys -t <pane> -X cancel'` ;
2. `load-buffer -b claude-tower-<n> -`, texte sur l'entrée standard ;
3. `paste-buffer -p -d -b claude-tower-<n> -t <pane>`, sans `-S` ;
4. 50 ms d'attente ;
5. `send-keys -t <pane> Enter`.

Un mutex **par volet** (table globale du paquet, clé socket + `%N`) empêche deux collages de s'entrelacer. Le tampon s'appelle `claude-tower-<pid>-<n>`. Si `paste-buffer` échoue, `delete-buffer` le supprime.

**Sessions de vue**
- `NewViewSession(pane)` :
  - note le volet actif de la fenêtre (`display-message -p -t <window> '#{pane_id}'`) ;
  - `new-session -d -t <SessionID> -s claude-tower-view-<pid>-<n>`, puis `set-option -t <id de la vue> status off` ;
  - `select-window -t <id de la vue>:<WindowID>`, puis `select-pane -t <pane>` ;
  - renvoie `ViewSession{ID, Name, PreviousActive}`.
  - Elle **ne pose pas** `destroy-unattached` (voir `ptyterm`).
- `RestoreActivePane` remet le volet noté s'il existe encore.
- `SweepViewSessions` tue les sessions `claude-tower-view-*` sans client.
- Toutes les cibles sont des identifiants (`$N`, `@N`, `%N`).

#### `internal/locate`

- `Tree` (PID → PPID) se lit une seule fois par cycle. Linux : `/proc/*/stat`, champ ppid à l'index 1 après la dernière `)`. macOS : `ps -A -o pid=,ppid=`.
- `Match(sessions, panes, tree) map[int]tmux.Pane` rattache une session au volet dont `pane_pid` est le PID de la session ou l'un de ses ancêtres (au plus 32 niveaux).
- `Ancestors(pid)` renvoie la chaîne des ancêtres.

#### `internal/hosting`

**Injection** : tout passe par une valeur `Hosting{SocketDir, ConfigDir, Self string; Tmux tmux.Runner; Bin string; Env []string; ClaudeConfigDir string}`, construite par `core.Options` ou par `cli`. `Self` est le chemin du binaire `claude-tower` utilisé pour `exec-env` : `os.Executable()` en vrai, le binaire de test en test. Les tests ne touchent donc jamais aux vrais dossiers. Le `TestMain` des tests d'intégration délègue `exec-env` à `cli`.

**Socket privée** : `NewSystem` renseigne `SocketDir` avec la valeur par défaut.
- Linux : `$XDG_RUNTIME_DIR/claude-tower/tmux.sock`, sinon `~/.local/state/claude-tower/tmux.sock`.
- macOS : `~/Library/Application Support/claude-tower/tmux.sock`.
- Le dossier est créé en `0700`. S'il existe, `Lstat` vérifie qu'il appartient à l'utilisateur, qu'il est en `0700` et que ce n'est pas un lien symbolique ; sinon, erreur.

**Configuration** (`~/.config/claude-tower/` ; macOS : `~/Library/Application Support/claude-tower/`)
- `config.json` : `{"claudeCommand": ["claude"], "tmuxTmpDir": "", "claudeConfigDir": ""}`. `claudeCommand[0]` est la commande (chemin absolu possible). Les éléments suivants sont des arguments placés avant ceux de l'utilisateur. `tmuxTmpDir` remplace `TMUX_TMPDIR` pour trouver les serveurs de l'utilisateur.
- `EnsureConfig()` écrit `tmux.conf`, et fait `source-file` si le serveur privé tourne. Elle est appelée par l'application au démarrage **et par `NewSession`**, avant de démarrer le serveur. Contenu :

```tmux
set -g status off
set -g prefix None
set -g mouse on
set -g history-limit 50000
set -g escape-time 0
set -g extended-keys on
set -g focus-events on
set -g default-terminal tmux-256color
set -g window-size latest
```

Si `infocmp tmux-256color` échoue, on remplace `tmux-256color` par `screen-256color`.

**Environnement de la session**
- `SessionEnv(base []string, configDir string) []string` : tout `base` sauf `TMUX`, `TMUX_PANE`, `TERM`, `TERM_PROGRAM`, `TERM_PROGRAM_VERSION`, `COLUMNS` et `LINES`. Pour une session lancée **depuis l'application**, on y pose `CLAUDE_CONFIG_DIR=<dossier lu par l'application>` (`$CLAUDE_CONFIG_DIR`, sinon la clé `claudeConfigDir` de `config.json`, sinon `~/.claude`). Pour `run`, la valeur de l'appelant est gardée telle quelle, et `LANG=C.UTF-8` (`en_US.UTF-8` sous macOS) si aucune locale UTF-8 n'est définie.
- `LoginShellEnv(ctx)` : `$SHELL -ilc 'env -0'`, avec un délai de 10 s. Il ne sert que si l'application est lancée sans `TERM` (lanceur graphique). Le résultat est fusionné **sous** l'environnement courant, puis mis en cache pour la vie du processus.
- **Transmission sans `argv`** :
  - l'environnement est écrit dans un fichier créé par `os.CreateTemp` dans le dossier `0700` de la socket privée, chaque variable séparée par un octet NUL. Il y a un fichier par tentative, supprimé sur tout chemin d'échec de `NewSession` ;
  - la commande de la session est `<chemin de claude-tower> exec-env <fichier> -- <claude résolu> <args…>` ;
  - `exec-env` (dans `internal/cli`) lit le fichier et le supprime. Il construit l'environnement final avec les variables du fichier, mais garde de son propre environnement `TERM`, `TMUX`, `TMUX_PANE` et `COLORTERM`, posées par tmux dans le volet. Puis `syscall.Exec`.
  - Seul le chemin du fichier apparaît dans les arguments.

**`NewSession(ctx, dir string, args []string, env []string) (Pane, error)`**
- **Résolution de claude** :
  1. `claudeCommand[0]` s'il est absolu et exécutable ;
  2. sinon `exec.LookPath` avec le `PATH` de `env` ;
  3. sinon `$SHELL -ilc 'command -v <nom>'` (10 s), dont la sortie doit être un chemin absolu exécutable ;
  4. sinon, erreur « claude introuvable : réglez claudeCommand dans <chemin> ».
- **Création** : `new-session -d -P -F '#{session_id}\t#{pane_id}' -x 200 -y 50 -c <dir> -s <nom> -- <commande exec-env…>`.
  - Le serveur est démarré avec `-f <tmux.conf>`.
  - Nom : `basename(dir)`, avec `.` et `:` remplacés par `-`.
  - Si tmux répond « duplicate session », on réessaie avec `-2`, `-3`, etc. (au plus 50 essais).
- **Vérification** : après 1 s, si le volet n'existe plus, erreur « la session s'est arrêtée immédiatement ».

**`Run(args []string) error`** (`claude-tower run`)
- Si `$TMUX` est défini : `syscall.Exec` du claude résolu, avec `args`.
- Sinon : `NewSession(cwd, args, SessionEnv(os.Environ(), configDir))`, puis `syscall.Exec` de `tmux -S <privé> attach -t <session_id>`.

**`Kill(ctx, pane Pane) error`**
- Refusé hors serveur privé.
- Tue d'abord les sessions de vue groupées avec la session, puis la session elle-même.

#### `internal/focus`

`Raise(pids []int) error` met au premier plan la fenêtre du premier PID de la liste qui en a une. `Supported() bool` dit à l'avance si c'est possible.
- **X11** (`DISPLAY` défini, `WAYLAND_DISPLAY` vide), avec `jezek/xgb` : on parcourt `_NET_CLIENT_LIST` et `_NET_WM_PID`, puis on envoie `_NET_ACTIVE_WINDOW` (source 2). Pour un terminal qui gère plusieurs fenêtres (gnome-terminal), c'est la première trouvée qui passe au premier plan : limite connue.
- **macOS** : le premier PID dont `ps -o comm= -p` contient `.app/`, puis `osascript -e 'tell application "System Events" to set frontmost of (first process whose unix id is <pid>) to true'`. L'erreur éventuelle est renvoyée telle quelle (autorisations).
- **Sinon** : `ErrUnsupported`.

#### `internal/core`

```go
type Card struct {
    PID, Order   int    // Order: index in StartedAt-then-PID order
    Name, State, Label, Since, Cwd, Activity string
    InTmux, Private, Attached, CanPrompt bool
    PromptReason string // French reason when CanPrompt is false
}
type Screen struct{ Lines []string; Width, Height int; Hash string } // Hash covers Lines, Width and Height
type FocusResult struct{ Mode, Warning string }                     // Mode: "external" | "embedded"

func New(opts Options) (*Core, error)               // checks tmux binary and version
func (c *Core) Cards(ctx) ([]Card, error)
func (c *Core) Screen(ctx, pid, maxLines int) (Screen, error)
func (c *Core) Prompt(ctx, pid int, text string) error
func (c *Core) Focus(ctx, pid int) (FocusResult, error)
func (c *Core) NewSession(ctx, dir string) error
func (c *Core) Kill(ctx, pid int) error
func (c *Core) PaneOf(pid int) (tmux.Pane, bool)    // from the last snapshot
```

**Instantané**
- `Cards` fait un cycle complet :
  1. le registre ;
  2. les serveurs (dossier tmux + socket privée) ;
  3. les volets et clients **en parallèle par serveur**, un serveur en échec étant ignoré ;
  4. un `Tree` ;
  5. `Match` ;
  6. `Finder.Retain` et `Finder.Last`.
- Le résultat est gardé comme **dernier instantané**, protégé par un mutex.

**`Screen`** : volet du dernier instantané, puis `Capture`, puis les `maxLines` dernières lignes non vides de l'écran (le `\n` final est retiré). Pas de nouveau cycle.

**`PromptReason`**, dans l'ordre :
1. « session hors tmux » ;
2. « attend une autorisation » (`waitingFor` = `permission prompt`) ;
3. « attend une réponse (<waitingFor>) » ;
4. « état inconnu » pour un statut autre que `idle`, `busy` ou `shell`.

**`Prompt`**
- Refuse un texte vide après `TrimSpace`.
- Relit **le seul fichier de registre** de la session juste avant `Paste` ; refuse si le statut n'est plus autorisé, ou si le volet n'existe plus.

**`Focus`** : une fonction pure testée seule.
- Dans tmux, au moins un client, et `focus.Supported()` : d'abord `Raise(Ancestors(PID du client))` sur le client d'`Activity` maximale, puis, **seulement si `Raise` réussit**, `SwitchClient`. Le résultat est `external`. Si `Raise` échoue : `embedded` + `Warning` (la raison), sans rien basculer.
- Dans tmux, sans client ou sans prise en charge : `embedded`, **sans basculer** aucun client.
- Hors tmux : `Raise(Ancestors(PID))`, ou l'erreur « impossible d'afficher le terminal de cette session ».

#### `internal/cli`

- `Main(args []string, …) int`, appelé par `main.go` quand `os.Args[1]` est une sous-commande.
- Si le premier argument est `run`, la suite est transmise **sans analyse** à `hosting.Run`. Même chose pour `exec-env`.
- `list` (avec une colonne `privé`, `tmux` ou `hors tmux`) et `bar` reprennent le code et les tests actuels de `main.go`. `list` fonctionne même sans tmux (colonne `?`).
- `internal/tui` et `internal/tmuxctl` sont supprimés.
- `main.go` au lot A : appelle `cli.Main`. Sans sous-commande, il affiche « interface graphique : lot B ».

#### Tests du lot A

**Unitaires**, avec un `Runner` injecté et les arguments exacts vérifiés :
- `Paste` : ordre des commandes, texte par l'entrée standard et absent des arguments, mutex ;
- `NewViewSession` (pas de `destroy-unattached`), `RestoreActivePane`, `SweepViewSessions` ;
- `NewSession` : identifiants, réessai, commande introuvable, résolution par le shell qui renvoie un alias (refusée) ;
- `Kill` (refus hors serveur privé, vues tuées d'abord) ;
- `SessionEnv`, `LoginShellEnv` (fusion), `PrivateSocket` (dossier d'un autre propriétaire, mauvais mode, lien symbolique : refusés), `FindBinary`, `CheckVersion` (`3.0a`, `3.1`, `3.2`, `3.2a`, `3.3a`, `3.8`, `next-3.9`, et `master` accepté) ;
- analyse `-F` avec déduplication et exclusion des vues ; `Match` ; analyse de `ps` ;
- décision de `Focus` ; ordre de `PromptReason` ; `Prompt` refusé quand le fichier de registre passe à `waiting` après `Cards` ;
- `cli` : `run --resume -c --model x` transmis tel quel, et `exec-env` (environnement appliqué, fichier supprimé) via une fonction d'exécution injectée ;
- tous les tests repris de `main_test.go` (`bar`, `list`, analyse des arguments).

**Intégration** avec un vrai tmux :
- socket dans `os.MkdirTemp("/tmp", "ct")`, `kill-server` et suppression du dossier en `t.Cleanup` ;
- tests sautés si `tmux` est absent.

La commande des volets de test est le **binaire de test lui-même**, en mode auxiliaire (variable d'environnement lue dans `TestMain`) : il passe le terminal en mode brut, active le collage entre crochets (`\e[?2004h`), puis écrit tous les octets reçus dans un fichier.

Cas couverts :
- `Paste` d'un texte de 50 Kio, d'un texte sur plusieurs lignes et d'un texte contenant ESC : le fichier contient `\e[200~`, le texte (ESC neutralisé, LF devenu CR), `\e[201~`, puis `\r` ;
- `Paste` sur un volet en copy-mode : même résultat ;
- `NewViewSession` : `Panes` ne renvoie le volet qu'une fois, le volet actif d'origine est restauré par `RestoreActivePane`, et `SweepViewSessions` tue une vue sans client ;
- `Servers` puis `Panes` sur une socket morte : ignorée ;
- `NewSession` avec le mode auxiliaire « écrire l'environnement » : une variable de l'appelant arrive, `TERM`, `TMUX` et `TMUX_PANE` sont présents, et le fichier d'environnement a disparu.

**`focus`** : seules les fonctions pures sont testées.

### Lot B (Sonnet, effort high) : application Wails et plein écran

Prérequis : lot 0 et lot A faits.

#### `main.go`

- Si `os.Args[1]` est une sous-commande connue : `os.Exit(cli.Main(...))`.
- Sinon, l'application : fenêtre `claude-tower` de 1400×900, et `//go:embed all:frontend/dist`.
- C'est le seul fichier qui importe Wails.
- Les liaisons `frontend/wailsjs/` sont générées par `wails generate module -tags webkit2_41`, puis committées.

#### `internal/app`

Les appels propres à Wails passent par deux interfaces : `Emitter` (`Emit(name string, data any)`) et `Dialogs` (`ChooseDirectory() (string, error)`).

Méthodes exposées au frontend :
- `Cards()`, `Screen(pid, maxLines)`, `Prompt(pid, text)`, `Focus(pid)`, `Kill(pid)` ;
- `NewSession()` : dialogue de choix du dossier, puis `core.NewSession`. Une annulation n'est pas une erreur ;
- `OpenTerminal(pid, cols, rows) (id, error)`, `TermReady(id)`, `TermInput(id, data)`, `TermResize(id, cols, rows)`, `CloseTerminal(id)`.

Cycle de vie :
- **Démarrage** : `core.New` (si tmux est absent ou trop ancien, l'erreur est affichée par le frontend), `EnsureConfig`, puis `SweepViewSessions` sur chaque serveur.
- **`OnShutdown`** : ferme tous les terminaux.

#### `internal/ptyterm`

**`Open`**
- `core.PaneOf`, puis `NewViewSession`.
- Puis `tmux -S <socket> attach -t <id de la vue> ";" set-option -t <id de la vue> destroy-unattached on` (le `;` est **un argument séparé** passé à `exec.Command`, pas la chaîne `\;`), dans un PTY (`creack/pty`) de la taille demandée.
- Environnement du PTY : celui de l'application, avec `TERM=xterm-256color`, `COLORTERM=truecolor`, et une locale UTF-8 si aucune.
- En cas d'échec après `NewViewSession` : `KillSession` de la vue et `RestoreActivePane`.
- L'identifiant est un compteur unique.

**Sortie vers le frontend**
- Les octets sont mis en **tampon jusqu'à `TermReady(id)`**. Ensuite, ils partent par l'évènement `term:<id>:data` (base64), par paquets d'au plus 16 Kio ou toutes les 16 ms.
- `term:<id>:exit` est émis après la sortie restante. Si la fin arrive avant `TermReady`, il est émis à `TermReady`.

**`Close`** : tue le processus, puis `KillSession` de la vue (sans erreur si elle a déjà disparu), puis `RestoreActivePane`.

**Tests**, avec un serveur tmux de test et un `Emitter` factice :
- rien n'est émis avant `TermReady`, puis le tampon arrive ;
- `echo $((6*7))` + Entrée fait apparaître `42` ;
- `TermResize` est appliqué ;
- `Close` fait disparaître la vue, après une **vraie attache** ;
- tuer le processus `attach` seul fait aussi disparaître la vue (`destroy-unattached`) ;
- un environnement vide fonctionne (TERM forcé) ;
- `internal/app` : l'annulation du dialogue ne crée pas de session.

### Lot C (Sonnet, effort high) : frontend

Prérequis : lot B fait.

#### Grille : mosaïque (comme un gestionnaire de fenêtres « tiling »)

- Les cartes occupent **toute** la zone au-dessus de la barre de prompt, sans marge perdue ni défilement tant que les cartes restent assez grandes.
- Ordre `Order` (fixe).
- Disposition par défaut : division verticale d'abord.
  - `cols = ceil(sqrt(n))`, `rows = ceil(n / cols)`.
  - Chaque ligne fait `1/rows` de la hauteur.
  - Chaque carte d'une ligne pleine fait `1/cols` de la largeur. La dernière ligne, si elle est incomplète, répartit sa largeur entre ses cartes.
  - Exemples : 1 session prend tout l'écran ; 2 sessions sont côte à côte (50 % de la largeur, 100 % de la hauteur) ; 3 sessions donnent deux cartes en haut et une pleine largeur en bas ; 4 donnent 2×2 ; 6 donnent 3×2.
- La disposition est calculée par une fonction pure `layout(n, width, height) → rectangles`, testée, pour pouvoir l'ajuster facilement après les essais réels.
- Garde-fou : en dessous de 320×180 px par carte, on passe au défilement vertical avec des lignes de 180 px.
- La sélection suit le PID. Si la session sélectionnée disparaît, on prend la voisine à la même position.

#### Carte

**En-tête et activité**
- En-tête : nom, libellé, durée, dossier. Puis une ligne d'activité.
- Le texte venant des sessions est toujours inséré comme texte, **jamais en `{@html}`**.

**Corps, session dans tmux**
- Un xterm.js en lecture seule : `disableStdin`, `scrollback: 0`, `addon-unicode11`, police de 11 px, `cols` et `rows` calculés pour remplir le corps de la carte.
- `Screen(pid, rows)` renvoie les dernières lignes.
- Écriture : `\x1b[?7l` (pas de retour à la ligne automatique, donc lignes coupées à droite) puis `\x1b[H`, puis chaque ligne suivie de `\x1b[K\r\n` (la dernière sans `\r\n`), puis `\x1b[J`.
- Mise à jour seulement si `Hash` change.

**Corps, session hors tmux** : les dernières lignes d'activité, et « hors tmux : lecture seule ».

**Cadre** : couleur selon l'état, plus épais sur la carte sélectionnée.

#### Rafraîchissement

- `Cards()`, puis `Screen()` des cartes visibles (`IntersectionObserver`), en série.
- Le cycle suivant est programmé 1 s **après la fin** du précédent (`setTimeout`).
- Pause quand la fenêtre est masquée (`document.hidden`) ou en plein écran.

#### Souris et clavier dans la grille

- **Clic** : sélectionne la carte.
- **Double-clic** ou `Entrée` : `Focus`. Si le résultat est `embedded`, ouvrir le plein écran. Afficher `Warning` dans un bandeau temporaire.
- **Flèches et `h` `j` `k` `l`** : déplacement selon la géométrie de la grille.
- `o` : plein écran (session dans tmux).
- `i` ou `/` : met le curseur dans la barre de prompt.
- `n` : nouvelle session.
- `x` : arrêter la session (privée seulement), avec confirmation qui affiche le nom et le PID.

#### Barre de prompt

- Zone de texte en bas : `Entrée` envoie, `Maj+Entrée` passe à la ligne, `Échap` revient à la grille.
- Désactivée quand `CanPrompt` est faux, avec `PromptReason` affichée.
- **Pendant l'envoi**, la zone est désactivée. En cas de succès, elle se vide. En cas d'erreur, le message s'affiche et le texte est gardé.

#### Plein écran

- xterm.js avec `addon-fit`, `addon-unicode11` et `addon-clipboard` (OSC 52).
- Séquence : `OpenTerminal`, puis `EventsOn("term:<id>:data")` et `EventsOn("term:<id>:exit")`, puis `TermReady(id)`.
- Les données sont décodées du base64 en `Uint8Array`, puis passées à `term.write(bytes)`.
- Les frappes passent par `TermInput` **en série** : un appel ne part qu'après le retour du précédent, et les frappes arrivées entre-temps sont regroupées. En effet, l'ordre d'exécution des appels liés de Wails n'est pas garanti. Les redimensionnements passent par `TermResize`. `Maj+Entrée` envoie `\x1b\r`.
- Un bouton et `Ctrl+Maj+Q` (jamais transmis à la session) reviennent à la grille : `CloseTerminal`, désabonnement, puis destruction du terminal.
- `term:<id>:exit` ramène aussi à la grille.

#### Thème et erreurs

- Thème sombre, police monospace du système.
- Une erreur de `core.New` (tmux absent ou trop ancien) s'affiche en page d'accueil, avec la commande d'installation.

#### Tests

La logique est dans des modules TypeScript purs, testés avec vitest :
- `layout` (1, 2, 3, 4, 5, 6 et 13 sessions ; dernière ligne incomplète ; garde-fou de taille minimale) ;
- navigation (bords, ligne incomplète, redimensionnement) ;
- suivi de la sélection, et disparition de la carte sélectionnée ;
- état de la barre de prompt (désactivée pendant l'envoi, texte gardé en cas d'erreur) ;
- classe du cadre selon l'état ;
- calcul de `cols` et `rows` du corps ;
- séquence d'écriture du corps ;
- programmation du rafraîchissement ;
- décodage base64 en octets, avec un caractère multi-octets coupé entre deux paquets ;
- séquence d'ouverture du plein écran, avec des liaisons factices.

En plus : `npm run check` et `npm run build`.

### Lot D (orchestrateur) : vérification réelle, documentation

**Vérifications sur le poste de l'utilisateur**, faites par l'utilisateur pour ce qui touche à ses sessions :
- la grille et la lisibilité des corps de carte ;
- un prompt (sur une ligne, sur plusieurs lignes, long) envoyé à une session lancée par l'application, pour confirmer que l'Entrée après le collage soumet bien ;
- `claude-tower run --resume` dans Alacritty, puis fermeture de la fenêtre : la session reste dans l'application ;
- `Entrée` sur une session de son tmux : bascule, premier plan, volet actif inchangé après le retour d'un plein écran ;
- plein écran, `Maj+Entrée`, copier-coller, retour ;
- arrêt d'une session privée ;
- lancement depuis un lanceur de bureau, sans `TERM` : sessions trouvées, nouvelle session fonctionnelle. Après fermeture de l'application, les sessions privées restent ;
- plein écran sur une session affichée dans son tmux : vérifier que le redimensionnement de sa fenêtre est acceptable (sinon, `attach -f ignore-size` pour les vues).

**Construction et documentation**
- Construction : `wails build -tags webkit2_41`.
- `Makefile` : `test` (Go `./internal/...` + build avec le tag + frontend), `build` et `install`.
- **README** :
  - installation (tmux 3.2 ou plus), `claude-tower run` et l'alias conseillé, `config.json` ;
  - touches ;
  - avec `mouse on`, `Maj` pour sélectionner du texte dans un terminal `run` ;
  - limites : Wayland, terminaux hors tmux, gnome-terminal, format interne du registre, secrets visibles dans les cartes, un brouillon déjà tapé dans le prompt de claude part avec le message collé, sous macOS un PID réutilisé peut faire réapparaître une session morte, sessions `run` qui survivent à leur terminal, binaire lié à GTK et WebKit (y compris pour `bar` et `run`) ;
  - **checklist macOS** pour le collègue testeur : installation, vivacité des sessions, `run`, premier plan (autorisations), socket privée, `tmux-256color`.

## Bloc commun des briefs (lots A, B, C)

> Dépôt : `/home/alexis/Projects/claude-tower`, branche `desktop` (base : `main`). Ne pas committer ni merger : l'orchestrateur committe après revue. Plan : `/home/alexis/Projects/claude-tower/PLAN.md` ; exécute uniquement le lot indiqué en fin de brief.
> Ne lance aucun sous-agent ni fork : fais le travail toi-même.
> N'installe ni ne télécharge aucune dépendance. Commandes Go avec `GOPROXY=off GOFLAGS=-mod=readonly`. Jamais de `go get`, `go mod tidy`, `npm install`, ni de commande `wails` qui télécharge. S'il manque une dépendance ou une entrée de `go.sum`, arrête-toi et signale-le.
> tmux : uniquement sur des sockets créées par les tests dans un dossier `os.MkdirTemp("/tmp", "ct")` (option `-S`), tuées en fin de test. Jamais de commande sur le serveur par défaut de l'utilisateur ni sur la socket privée de claude-tower.
> Ne lis et n'écris rien dans `~/.claude`, `~/.claude-perso` ni `~/.claude-shared`. Les tests utilisent des fixtures inventées.
> Commentaires de code en anglais. Ils décrivent le comportement actuel, au présent, et se comprennent en lisant le code seul : pas de référence externe (ticket, plan, PR), pas de date, pas d'historique. Pas de commentaire qui paraphrase le code : n'expliquer que ce que le code ne dit pas (contrainte, invariant, raison d'un choix). Une doc comment Go par identifiant exporté.
> Go idiomatique, fonctions courtes, `gofmt`. TypeScript strict.
> Le shell a des alias interactifs (`rm -i`, `ls` = eza, `cat` = bat) : utilise `command rm`, `command ls`, `command cat`.
> Tests : `cd /home/alexis/Projects/claude-tower && GOPROXY=off GOFLAGS=-mod=readonly go vet ./internal/... && GOPROXY=off GOFLAGS=-mod=readonly go test ./internal/... && GOPROXY=off GOFLAGS=-mod=readonly go build -o /dev/null .` (à partir du lot B : `go build -tags webkit2_41 -o /dev/null .`). À partir du lot C, en plus : `cd frontend && npx vitest run && npm run check && npm run build`.
> Rapport final : fichiers créés ou modifiés, sortie des tests, écarts au plan et pourquoi.

## Revues

Pour chaque lot :
- l'orchestrateur relit le diff et lance les tests ;
- une revue fraîche Sonnet, qui ne reçoit que l'objectif et l'emplacement du changement ;
- une re-revue des seuls correctifs.

Un commit par lot sur `desktop`.
