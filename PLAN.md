# claude-tower : plan d'implémentation

Outil Go en lecture seule pour suivre les sessions Claude Code en cours sur la machine :

- `claude-tower bar` : segment pour la barre d'état tmux ;
- `claude-tower` : interface en terminal (Bubble Tea) dans un `tmux display-popup`, où `Entrée` amène sur le volet de la session.

Contraintes : pas de hook, pas de serveur, pas de base de données, pas d'écriture hors du dépôt. On affiche le strict nécessaire (nom, état, durée, dossier, dernière activité) : pas de tokens, pas de statistiques. Un seul compte Claude : le compte courant.

## Faits établis (vérifiés le 2026-11-09, Claude Code 2.1.318)

### Registre des sessions

Il y a un fichier `<configDir>/sessions/<pid>.json` par session vivante. Le fichier est supprimé quand la session se termine proprement. Le même dossier contient aussi des fichiers `<pid>.<hash>.key` : ne jamais les lire. Exemple de fichier :

```json
{"pid":824693,"sessionId":"75f334f0-4dc9-41db-85e2-1d96e0dbaaf1","cwd":"/home/alexis/zest",
 "startedAt":1794216804002,"procStart":"136546471","version":"2.1.318","kind":"interactive",
 "entrypoint":"cli","tmux":"0:@0.%0","messagingSocketPath":"/run/user/1000/cc-socks/824693.sock",
 "name":"zest-fe","nameSource":"derived","status":"idle","updatedAt":1794222510978,
 "statusUpdatedAt":1794222510978}
```

- Les horodatages sont en millisecondes depuis l'epoch.
- `status` prend les valeurs `busy`, `shell`, `idle` et `waiting` ; c'est l'énumération du registre dans le binaire.
  - `shell` : la session exécute une commande shell.
  - `waiting` : la session attend l'utilisateur, avec un champ `waitingFor` (valeurs vues dans le binaire : `"dialog open"`, `"input needed"`, `"sandbox request"`, `"worker request"`, `"goal proposal"` ; la liste n'est pas fermée). Une demande d'autorisation donne `"permission prompt"` (observé le 2026-11-09).
- `kind` : `interactive`, `bg`, `daemon` ou `daemon-worker`. Il peut aussi y avoir un booléen `spare`.
- `entrypoint` : `cli` pour une session interactive. Un `claude -p` s'inscrit pendant son exécution avec `kind: interactive` et `entrypoint: sdk-cli`, et hérite du volet tmux de son parent : les `entrypoint` qui commencent par `sdk` sont ignorés.
- `tmux` est au format `<session>:@<window_id>.%<pane_id>`. Il peut manquer si la session tourne hors de tmux. Il ne contient pas la socket du serveur tmux.
- `procStart` (chaîne de chiffres aujourd'hui) est égal au champ 22 de `/proc/<pid>/stat` (starttime, en ticks). Comparer les deux permet de détecter un fichier orphelin dont le PID a été réutilisé. Le champ `comm` (2) peut contenir des espaces et des parenthèses : découper après la **dernière** `)`. Dans le reste de la ligne découpé par `strings.Fields`, le champ 3 (state) est à l'index 0, donc starttime est à l'**index 19**.
- Il peut y avoir d'autres champs (`pidDomain`, `peerFeatures`, `logPath`…) : on les ignore.
- Claude Code ne réécrit peut-être pas ce fichier de façon atomique : une lecture peut tomber sur un JSON tronqué.
- Ce format est interne et non documenté : tout ce qui en dépend vit dans le seul paquet `internal/registry`.

### Transcript

Chemin : `<configDir>/projects/<slug>/<sessionId>.jsonl`, une ligne JSON par enregistrement. On le trouve par glob `projects/*/<sessionId>.jsonl`, sans recalculer le slug. Champs utiles :

- `type` : on ne retient que `assistant` et `user`. Les autres (`system`, `attachment`, `api-request`, `api-request-blob`, `mode`, `last-prompt`…) sont ignorés.
- `isMeta: true` : message injecté par l'outil (rappels, messages d'agents), à ignorer.
- `isCompactSummary: true` ou `isVisibleInTranscriptOnly: true` : résumé écrit après un compactage (`type: user`, contenu en chaîne), à ignorer.
- `message.content` : soit une chaîne (prompt tapé), soit un tableau de blocs `{type: "text"|"thinking"|"tool_use"|"tool_result"|"image", ...}`.
  - Un bloc `tool_use` porte `name` et `input`. Selon l'outil, l'argument utile est `input.command` (Bash), `input.file_path` (Read, Edit, Write), `input.pattern` (Grep, Glob) ou `input.description` (Agent).
- Les chaînes `user` qui commencent par `<` sont des notifications ou commandes internes (`<task-notification>`, `<command-name>`, `<local-command-caveat>`…), à ignorer. Les textes `user` qui commencent par `[Request interrupted` ou `[Image` sont du bruit, à ignorer aussi.
- Certaines lignes dépassent plusieurs centaines de Kio (blobs, gros `tool_result`, images en base64).
- Une session peut ne pas avoir encore de transcript (session neuve, `/clear`) : la dernière activité est alors vide, ce n'est pas une erreur.

### Environnement

- Go 1.26. `go.mod` (module `claude-tower`) et `go.sum` contiennent déjà bubbletea v1.3.10 et lipgloss v1.1.0, ajoutés depuis le cache.
- Les dépendances de test de lipgloss (`go-udiff`, `x/exp/golden`) ne sont pas dans le cache : `go mod tidy` hors ligne échouera. Ne pas le lancer.
- `github.com/charmbracelet/x/ansi` v0.10.1 (dépendance de lipgloss) est utilisable directement, y compris au lot 1 : `ansi.Truncate(s, width, tail)` et `ansi.StringWidth(s)`.
- tmux 3.8. Config `~/.config/tmux/tmux.conf`, avec tpm et les plugins tmux-sensible, tmux-autoreload, tmux-resurrect et tmux-dotbar ; la ligne `run '…/tpm/tpm'` est la dernière.
  - `status-interval` vaut déjà 5 (tmux-sensible).
  - tmux-dotbar écrit `status-right` au chargement, à partir de `@tmux-dotbar-right` et `@tmux-dotbar-status-right`.
  - tmux-autoreload recharge la config à chaque modification. Un `set -ga` y serait donc ré-appliqué à chaque rechargement.
  - `status-right-length` vaut 40 ; `status-justify` vaut `absolute-centre`.
  - `prefix C` est lié à `customize-mode`. `prefix a` est libre.
  - Les commandes `#(...)` et `display-popup` héritent de l'environnement du serveur tmux : chemins absolus obligatoires.

## Arborescence cible

```
~/Projects/claude-tower/
├── go.mod, go.sum
├── main.go                     # sous-commandes : (aucune) → TUI, bar, list ; flags --dir, --client
├── Makefile                    # build, test, install (→ ~/.local/bin, lancé par l'utilisateur)
├── README.md
├── internal/registry/          # lecture du registre, vivacité, filtrage, normalisation de l'état
├── internal/transcript/        # dernière activité d'une session
├── internal/view/              # formatage partagé : icône, libellé, durée, tri, nom affiché, barre
├── internal/tui/               # Bubble Tea (lot 2)
└── internal/tmuxctl/           # saut vers un volet (lot 2)
```

## Étapes

### Lot 0 : initialisation et dépendances (fait)

`git init` (branche `main`), `go mod init claude-tower`. L'utilisateur a ajouté bubbletea et lipgloss depuis le cache. La compilation hors ligne d'un fichier qui importe bubbletea, lipgloss et x/ansi réussit (`GOPROXY=off GOFLAGS=-mod=readonly`).

Dans `go.mod`, toutes les dépendances sont marquées `// indirect`, même celles importées directement. C'est sans effet sur `go build`, `go vet` et `go test` : ne pas chercher à le corriger.

L'orchestrateur ajoute un `.gitignore` (binaire `claude-tower` à la racine), puis committe `PLAN.md`, `.gitignore`, `go.mod` et `go.sum` avant le lot 1.

### Lot 1 (Sonnet, effort medium) : registre, transcript, `bar` et `list`

#### `internal/registry`

```go
type State int // StateUnknown (= iota, zero value), StateBusy, StateWaiting, StateIdle

type Session struct {
    PID         int
    SessionID   string
    Name        string    // raw registry "name", may be empty
    Cwd         string
    Kind        string
    State       State
    RawStatus   string    // "busy", "shell", "idle", "waiting", or anything else
    WaitingFor  string    // registry "waitingFor", "" when absent
    StartedAt   time.Time // zero if absent
    StatusSince time.Time // statusUpdatedAt, else updatedAt, else zero
    TmuxPane    string    // "%N" extracted from "tmux", "" if absent or malformed
    TurnEnded   bool      // never set by registry; filled by callers from transcript.Activity
}

type Reader struct {
    Dir      string                                  // config dir, e.g. /home/alexis/.claude
    Alive    func(pid int, procStart string) bool    // default: procAlive; injectable for tests
    ReadFile func(path string) ([]byte, error)       // default: os.ReadFile; injectable for tests
}

func (r Reader) Sessions() ([]Session, error)
```

**Lecture**
- Lire uniquement les fichiers de `<Dir>/sessions/` dont le nom correspond à `^[0-9]+\.json$`.
- Si le dossier `sessions/` n'existe pas, renvoyer une liste vide, sans erreur. `Sessions` ne renvoie une erreur que si le dossier existe et ne peut pas être lu (droits, etc.).
- Un fichier dont le JSON ne se décode pas est relu une fois après 20 ms, car une réécriture est peut-être en cours. S'il échoue encore, on ignore la session.
- Décodage tolérant : `procStart` est accepté en chaîne ou en nombre (`json.RawMessage`, puis normalisé en chaîne de chiffres), les horodatages en nombre entier ou flottant. Un champ de type inattendu vaut zéro au lieu de faire échouer tout le fichier.
- Le PID vient du nom de fichier. Si le JSON contient un `pid` présent et non nul qui en diffère, on ignore le fichier.
- Assainissement à la lecture : dans `Name`, `Cwd`, `Kind`, `RawStatus` et `WaitingFor`, les caractères de contrôle (`unicode.IsControl`, ESC compris) sont retirés. `WaitingFor` est en plus limité à 32 runes.

**Vivacité et filtrage** : on ignore la session dans les cas suivants.
- Le processus n'est pas vivant. `procAlive` vérifie que `/proc/<pid>/stat` existe et que son champ 22 vaut `procStart` ; si `procStart` est vide, l'existence du processus suffit. La découpe de la ligne se fait dans une fonction pure `parseStartTime(statLine string) (string, bool)` (index 19 après la dernière `)`, voir plus haut).
- `kind` vaut `daemon` ou `daemon-worker`, ou `spare` vaut `true`. Un `kind` vide, `interactive`, `bg` ou toute autre valeur est conservé.

**Normalisation de l'état**

| `status` | `State` |
| --- | --- |
| `busy`, `shell` | `StateBusy` |
| `waiting` | `StateWaiting` |
| `idle` | `StateIdle` |
| toute autre valeur | `StateUnknown` (on garde `RawStatus`) |

**Volet tmux** : `TmuxPane` est la partie après le dernier `.`, seulement si elle correspond à `^%[0-9]+$`.

Le tri n'est pas le rôle de ce paquet : voir `view`.

#### `internal/transcript`

```go
type Finder struct { /* cache sessionID → path, taille, mtime, texte ; mutex */ }

type Activity struct {
    Text      string // "" when nothing retained
    TurnEnded bool   // the last turn-relevant record is an end-of-turn marker
}

func NewFinder(configDir string) *Finder
func (f *Finder) Last(sessionID string) Activity // zero Activity when unknown or not found
```

**Recherche du fichier**
- `sessionID` doit correspondre à `^[0-9a-fA-F-]{8,64}$`, sinon renvoyer `""` sans chercher.
- Recherche sans `filepath.Glob` (le dossier de config pourrait contenir `[`, `*` ou `?`) : `os.ReadDir(<configDir>/projects)`, puis `os.Stat` de `<sous-dossier>/<sessionID>.jsonl` pour chaque sous-dossier. On garde le premier trouvé.
- On ne met en cache un chemin qu'une fois le fichier trouvé : un transcript absent est recherché de nouveau à l'appel suivant.
- Si le `stat` d'un chemin en cache échoue (fichier supprimé ou renommé), on oublie l'entrée et on refait la recherche.
- On ne relit le fichier que si sa taille ou son mtime a changé ; sinon on renvoie le texte en cache.
- `Finder` est protégé par un `sync.Mutex`, car la TUI l'appelle depuis des `tea.Cmd`.
- Points d'injection pour les tests, en champs non exportés initialisés par `NewFinder` : la liste des tailles de fenêtre (`[]int64{256 << 10, 1 << 20, 4 << 20}` par défaut, quelques dizaines d'octets en test) et la fonction de lecture de la fin du fichier, qui peut compter ses appels. Les tests de cache changent le mtime avec `os.Chtimes`.

**Lecture de la fin du fichier**
- Lire une fenêtre de fin de fichier de la première taille. Si aucun enregistrement retenu n'y est trouvé et que le début du fichier n'est pas atteint, recommencer avec la taille suivante. Au-delà de la dernière, renvoyer `""`.
- Quand la fenêtre ne commence pas au début du fichier, la lecture démarre un octet plus tôt (`offset-1`) pour connaître l'octet précédent. La première ligne n'est écartée que si cet octet n'est pas `\n`.
- Parcourir les lignes de la fin vers le début. Les lignes non JSON sont ignorées.

**Choix de l'enregistrement** : le premier enregistrement retenu donne le texte.
- Ignorer tout enregistrement dont `type` n'est ni `assistant` ni `user`, et tout enregistrement avec `isMeta`, `isCompactSummary` ou `isVisibleInTranscriptOnly` à `true`.
- `assistant` dont le contenu a au moins un `tool_use` : on prend le dernier `tool_use` du message et on affiche `"<Name>: <arg>"`. L'argument dépend de l'outil :
  - Bash : `command` ;
  - Read, Edit, Write : `file_path` ;
  - Grep, Glob : `pattern` ;
  - Agent : `description` ;
  - autre outil, ou argument absent : seulement `"<Name>"`.
- `assistant` sans `tool_use`, avec un bloc `text` non vide : la première ligne non vide du dernier bloc `text`.
- `user` dont `content` est une chaîne qui, une fois les blancs de début retirés, ne commence ni par `<`, ni par `[Request interrupted`, ni par `[Image` : `"› " + première ligne non vide`.
- `user` dont `content` est un tableau contenant un bloc `text` qui satisfait la même règle : même chose avec ce bloc. Un tableau qui ne contient que des `tool_result`, des images ou des textes exclus est ignoré.
- Tout le reste est ignoré.

**Fin de tour (`TurnEnded`)** : pendant le même parcours de la fin vers le début, on regarde le premier enregistrement qui est soit `assistant`, soit `user` (y compris `isMeta` et résumés de compactage), soit `system` avec `subtype` égal à `turn_duration`. `TurnEnded` vaut `true` seulement si c'est ce `system/turn_duration`. Contexte : Claude Code laisse le registre en `busy` tant que des shells ou agents tournent en arrière-plan, même quand le tour est fini et que la session attend un message ; le marqueur `turn_duration` est écrit à chaque fin de tour.

**Mise en forme** : remplacer toute suite de blancs ou de caractères de contrôle par une seule espace, puis retirer les espaces de début et de fin. Pas de troncature à ce niveau : elle dépend de la largeur d'affichage.

#### `internal/view`

**Formatage**
- `type Display int` : `DisplayUnknown` (valeur nulle), `DisplayWaiting`, `DisplayBusy`, `DisplayBackground`, `DisplayIdle`. `DisplayOf(s registry.Session) Display` : Busy avec `TurnEnded` → Background ; sinon la correspondance directe de `State`.
- `Icon(Display) string` : Busy `●`, Waiting `!`, Background `◐`, Idle `○`, Unknown `?`.
- `Label(s registry.Session) string` : Busy → `travaille` (`shell` si `RawStatus` vaut `shell`) ; Waiting → `attend` suivi de ` (<WaitingFor>)` si non vide ; Background → `fini · fond` ; Idle → `fini` ; Unknown → `RawStatus`, ou `?` s'il est vide.
- `Duration(since, now time.Time) string`. Retourne `""` si `since` est nul. Les durées négatives valent 0. Paliers :
  - `<60s` → `"42s"` ;
  - `<60m` → `"7m"` ;
  - `<24h` → `"1h05"` ;
  - sinon `"3j"`.
- `DisplayName(s registry.Session) string` :
  - nom de base : `Name` s'il est non vide ; sinon `filepath.Base(Cwd)` si `Cwd` est non vide et que le résultat n'est ni `"."` ni `"/"` ; sinon `"pid <PID>"` ;
  - les caractères de contrôle sont retirés ;
  - le nom est tronqué à 16 colonnes d'affichage avec `ansi.Truncate(nom, 16, "…")`.
- `ShortPath(cwd, home string) string` : si `home` est non vide et que `cwd == home`, renvoie `~` ; si `cwd` commence par `home + "/"`, remplace ce préfixe par `~/`. Sinon `cwd` inchangé.

**Tri**
- `SortByName(ss)` : `DisplayName` sans tenir compte de la casse, puis `StartedAt`, puis `PID`. C'est l'ordre de la barre, stable d'un rafraîchissement à l'autre.
- `SortByUrgency(ss)` : selon `DisplayOf`, Waiting, Busy, Unknown, Background, Idle, puis le même ordre que `SortByName`. C'est l'ordre du popup.
- Les deux fonctions trient une copie et ne modifient pas leur argument.

**`Bar(ss []registry.Session, now time.Time, maxWidth int) string`** : sessions triées par nom, séparées par deux espaces, au format de style tmux.
Le style dépend de `DisplayOf` :
- Busy : `#[fg=yellow]● <nom> <durée>#[default]` ;
- Background : `#[fg=cyan]◐ <nom>#[default]` (sans durée) ;
- Waiting : `#[fg=red,bold]! <nom> <durée>#[default]` ;
- Idle : `#[fg=colour244]○ <nom>#[default]` (sans durée) ;
- Unknown : `#[fg=colour244]? <nom>#[default]`.
- Si la durée est vide, on n'ajoute pas l'espace qui la précède.
- Dans les noms, échapper `#` en `##`.
- Aucune session : chaîne vide.
- Largeur : on mesure la largeur visible (sans les `#[...]`, `##` comptant pour 1). Si elle dépasse `maxWidth`, on regroupe d'abord toutes les sessions Idle, Background et Unknown en un seul élément `#[fg=colour244]○ <n>#[default]` placé en fin de barre. Si c'est encore trop large, on regroupe aussi les Busy en `#[fg=yellow]● <n>#[default]`, placé juste avant. Les sessions Waiting restent toujours affichées une par une. `maxWidth <= 0` désactive la limite.

#### `main.go`

**Ligne de commande**
- Une fonction pure `parseArgs(args []string) (opts, error)` : le premier argument qui n'est ni un flag ni la valeur d'un flag est la sous-commande. Les flags `--dir`, `--max-width` et `--client` (déclaré dès le lot 1, utilisé seulement par la TUI au lot 2) sont acceptés avant comme après la sous-commande. Implémentation : un seul `flag.FlagSet` (en `flag.ContinueOnError`, sortie redirigée), appelé une première fois, puis une seconde fois sur les arguments restants après la sous-commande.
- Plus d'un argument positionnel : erreur d'usage.
- `main` se réduit à `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))`. La fonction `run(args []string, stdout, stderr io.Writer, getenv func(string) string) int` fait tout le reste et se teste directement.

**Dossier lu**
- `--dir <chemin>` si le flag est fourni ;
- sinon `$CLAUDE_CONFIG_DIR` s'il est non vide ;
- sinon `~/.claude`.

Un seul dossier : pas de liste, pas d'autre variable d'environnement.

**Sous-commandes**
- `claude-tower bar` : écrit `view.Bar(…, maxWidth)` puis un saut de ligne, avec `--max-width` (100 par défaut). Elle n'appelle `Finder.Last` que pour les sessions Busy, afin de remplir `TurnEnded` ; jamais pour les autres. En cas d'erreur, rien sur stdout, le message sur stderr, et le code de sortie est 0, pour ne pas polluer la barre tmux.
- `claude-tower list` : une ligne par session, en texte brut, triée par urgence, pour déboguer. Chaque ligne contient l'icône, le nom affiché, le libellé, la durée, le volet, le cwd raccourci et la dernière activité.
- Sans sous-commande : au lot 1, comme `list` ; le lot 2 branche la TUI à la place.
- Sous-commande inconnue : message d'usage sur stderr, code 2.

#### Tests

Bibliothèque standard `testing`, fixtures inventées dans `testdata/` ou construites dans `t.TempDir()`, sans donnée réelle. `Alive`, `ReadFile` et `now` sont injectés.

**Registre**
- fichier valide, avec et sans `waitingFor` ;
- chaque valeur de `status` : `busy`, `shell`, `idle`, `waiting` et une valeur inconnue ;
- JSON invalide, avec et sans succès à la relecture ;
- fichier `.key` ignoré ;
- dossier `sessions/` absent ;
- processus mort ou `procStart` différent ;
- `kind` `daemon` ou `daemon-worker`, et `spare: true` ;
- `tmux` absent ou malformé ;
- `statusUpdatedAt` absent ;
- `pid` du JSON différent du nom de fichier ; `procStart` en nombre ; champ de type inattendu ;
- caractères de contrôle (ESC) dans `Name`, `Cwd` et `WaitingFor` ; `WaitingFor` trop long ;
- `parseStartTime` sur une ligne `/proc/pid/stat` littérale dont le `comm` contient `") ("` et des espaces, avec la valeur attendue écrite en dur ; ligne trop courte ;
- `procAlive` sur le PID du test (`os.Getpid()`) : vrai avec le starttime de `/proc/self/stat`, faux avec une valeur différente, faux pour un PID inexistant.

**Transcript**
- `tool_use` Bash, Read et Agent, outil inconnu, plusieurs `tool_use` dans un même message ;
- texte assistant sur plusieurs lignes ;
- prompt utilisateur en chaîne et en tableau avec un bloc `text` ;
- enregistrements à ignorer : `isMeta`, `isCompactSummary`, chaîne commençant par `<task-notification>`, `[Request interrupted by user]`, `[Image: …]`, tableau ne contenant que des `tool_result`, types non retenus, ligne non JSON ;
- `TurnEnded` : vrai quand le dernier enregistrement pertinent est `system/turn_duration` (même suivi d'enregistrements `attachment`, `mode`, etc.) ; faux quand un `assistant` ou un `user` (même `isMeta`) vient après ; faux sans transcript ;
- avec de petites fenêtres injectées : première ligne de la fenêtre tronquée (écartée) ; fenêtre qui commence pile au début d'une ligne (gardée) ; dernier enregistrement plus long que la première fenêtre (trouvé grâce à l'élargissement) ; rien de retenu même dans la plus grande fenêtre (`""`) ;
- transcript absent, puis créé : il est trouvé à l'appel suivant ;
- transcript supprimé après avoir été mis en cache, puis recréé ailleurs : nouveau glob ;
- cache : pas de relecture si le fichier n'a pas changé, relecture après une modification (`os.Chtimes`) ;
- `sessionID` invalide.

**View**
- durées aux limites (59s, 60s, 59m, 60m, 24h), `since` nul, durée négative ;
- `DisplayName` : repli sur le cwd puis le PID (`Cwd` vide et `Cwd == "/"`), caractères de contrôle, troncature avec des caractères multi-octets ;
- `ShortPath` : `cwd == home`, sous-dossier de `home`, préfixe trompeur (`home=/home/alex`, `cwd=/home/alexis/zest`), `home` vide ;
- ordres de tri avec des noms égaux et des casses différentes ;
- `State` nul vaut `StateUnknown` ;
- `DisplayOf` : Busy avec et sans `TurnEnded`, Idle avec `TurnEnded` (reste Idle) ;
- `Bar` vide, échappement de `#`, absence de durée ;
- `Bar` avec cinq sessions et une petite `maxWidth` : regroupement des Idle, puis des Busy, Waiting toujours visibles.

**main**
- `parseArgs` : `bar --dir X`, `--dir X bar`, `--dir X --client Y` sans sous-commande, sous-commande inconnue, deux positionnels.
- `run` avec un dossier de config construit dans `t.TempDir()` : `bar` écrit la barre attendue ; `bar` sur un dossier `sessions/` illisible (`chmod 000`) sort en 0 avec stdout vide et un message sur stderr ; `bar` affiche `◐` pour une session `busy` dont le transcript se termine par `system/turn_duration`, et `●` sinon ; un transcript absent ou illisible laisse `●` ; `list` écrit une ligne par session ; sous-commande inconnue sort en 2 ; `$CLAUDE_CONFIG_DIR` (via `getenv`) est utilisé sans `--dir`, et `--dir` l'emporte.

### Lot 2 (Sonnet, effort medium) : TUI et saut tmux

#### `internal/tmuxctl`

```go
func Jump(client, pane string, sessionPID int) error
```

- `pane` vide ou ne correspondant pas à `^%[0-9]+$` : renvoyer une erreur explicite (`session hors tmux`).
- Si `client` commence par `#{`, c'est un format tmux non développé : le traiter comme vide.
- Vérifier d'abord que le volet existe sur ce serveur et qu'il est bien celui de la session : `tmux display-message -p -t <pane> '#{pane_pid}'` doit réussir, et ce PID doit être `sessionPID` ou l'un de ses ancêtres. On remonte la chaîne des parents via le champ 4 (ppid) de `/proc/<pid>/stat`, soit l'index 1 après la dernière `)`, au plus 32 niveaux. Sinon, renvoyer l'erreur `volet introuvable sur ce serveur tmux`. Cela couvre les identifiants `%N` d'un autre serveur tmux.
- Puis lancer une seule commande, `tmux switch-client [-c <client>] -t <pane>`, qui change à la fois de session, de fenêtre et de volet.
- Les commandes passent par `exec.Command`, sans shell. Deux variables de paquet sont injectables pour les tests : `runCmd` (exécution d'une commande, renvoie la sortie) et `parentPID` (lecture du ppid).

#### `internal/tui`

Bibliothèques : bubbletea v1.3.10, lipgloss v1.1.0.

**Modèle**
- Contenu : sessions triées par `SortByUrgency`, activité par PID, `now` du dernier chargement, PID sélectionné, décalage de défilement, largeur, hauteur, client, intervalle de rafraîchissement (champ non exporté, 1 s par défaut, réduit en test), et deux erreurs distinctes : `jumpErr` (saut) et `loadErr` (chargement).
- Deux fonctions injectées, fournies par `main.go` en vrai et par les tests en faux :
  - `load func() (loadedMsg)`, où `loadedMsg` contient les sessions, les activités par PID, `now` (pris au moment du chargement) et une erreur éventuelle. Elle est appelée dans un `tea.Cmd`. La vraie version utilise `Reader.Sessions` puis `Finder.Last` pour chaque session (texte d'activité, et `TurnEnded` recopié dans la session).
  - La TUI garde un seul `Finder` pendant toute sa vie. Ajouter à `transcript` une méthode `func (f *Finder) Retain(sessionIDs []string)` qui retire du cache les sessions absentes de la liste ; la vraie `load` l'appelle à chaque chargement avec les `SessionID` courants. Avec son test.
  - `jump func(client, pane string, pid int) error`, appelée dans un `tea.Cmd` qui renvoie un `jumpDoneMsg{err}`. La vraie version est `tmuxctl.Jump`.
- `Update` ne fait aucune entrée-sortie, et `View` n'appelle jamais `time.Now()` : les durées utilisent le `now` du modèle.
- Rafraîchissement : `Init` lance le premier chargement. À la réception de `loadedMsg`, le modèle se met à jour puis arme un `tea.Tick` de l'intervalle, dont le message (`tickMsg`) lance le chargement suivant. Il n'y a donc jamais deux chargements en cours.

**Sélection**
- La sélection suit le PID, pas l'index.
- Si la session sélectionnée disparaît, on sélectionne celle qui occupe maintenant le même index, borné à la fin de la liste.
- Tant qu'aucun PID n'est sélectionné et que la liste n'est pas vide, on sélectionne la première session.

**Affichage**
- Le programme tourne en écran alternatif (`tea.WithAltScreen()`).
- Chaque session occupe deux lignes :
  1. `<n> <icône> <nom affiché>  <libellé>  <durée>  <cwd raccourci>`, où `<n>` est le rang de 1 à 9 dans la liste triée complète (deux espaces au-delà du 9e) ;
  2. `  └ <dernière activité>`, ou une ligne vide si l'activité est vide.
- Les deux lignes sont tronquées à la largeur en largeur d'affichage (`github.com/charmbracelet/x/ansi`, `ansi.Truncate` avec `…`), jamais avec `len`.
- Tant qu'aucun `tea.WindowSizeMsg` n'est arrivé, la largeur vaut 80.
- Couleurs : les mêmes que la barre, via lipgloss. La ligne sélectionnée est en inversé.
- Défilement : le nombre de sessions visibles vaut `max(1, (hauteur - 2) / 2)`, et le décalage s'ajuste pour que la sélection reste visible.
- Pied de page : `↑↓ choisir · 1-9 sélection · Entrée aller · q fermer`, remplacé par `jumpErr` s'il existe, sinon par `loadErr` s'il existe. `jumpErr` s'efface à la touche suivante, jamais sur un `loadedMsg`. `loadErr` est remplacé par l'erreur de chaque `loadedMsg` (donc effacé au chargement réussi suivant).
- `View` ne dépasse jamais la hauteur : si la hauteur ne permet pas d'afficher une session et le pied de page, on tronque la sortie aux `hauteur` premières lignes.
- Aucune session : afficher `Aucune session Claude en cours.`

**Touches**
- `↑`/`k` et `↓`/`j` : déplacer la sélection.
- `1`…`9` : sélection de la session de ce rang dans la liste triée complète (le numéro affiché). Rang au-delà de la liste : rien. Ces touches sélectionnent seulement, elles ne déclenchent pas de saut.
- `Entrée` : sans sélection, rien. Sinon, appeler `jump(client, pane, pid)` dans un `tea.Cmd`. Sur `jumpDoneMsg` sans erreur, quitter (`tea.Quit`). Avec une erreur, afficher le message dans le pied de page et rester ouvert.
- `q`, `Échap`, `Ctrl-C` : quitter.

**`main.go`** : sans sous-commande, lancer la TUI. Ajouter le flag `--client`, transmis à `Jump`.

**Tests** (sans terminal)
- `Update` avec des messages clavier, de taille et de chargement ;
- avec un intervalle réduit : la commande renvoyée après `loadedMsg` produit un `tickMsg` ; celle renvoyée après `tickMsg` appelle `load` ;
- `jumpErr` survit à un `loadedMsg` ;
- suivi de la sélection après un retri ;
- disparition de la session sélectionnée ; liste vide puis non vide ;
- défilement, y compris avec une hauteur inférieure à 4 ;
- chiffres dans et hors limites ;
- `Entrée` : succès suivi de `tea.Quit` ; erreur affichée et popup resté ouvert, puis effacée à la touche suivante ; liste vide ;
- erreur de chargement affichée puis effacée ;
- troncature avec des caractères multi-octets ;
- `View` sans session, et `View` déterministe pour un `now` donné ;
- `Jump` : arguments passés à `runCmd`, client vide ou non développé, volet invalide, échec de `display-message`, `pane_pid` ancêtre de la session (succès), `pane_pid` sans lien (erreur), chaîne de parents interrompue.

### Lot 3 (orchestrateur) : vérifications réelles et branchement tmux

- **Makefile** : cibles `build`, `test` (`go vet ./... && go test ./...`) et `install` (`go build -o ~/.local/bin/claude-tower .`). C'est l'utilisateur qui lance `install`.
- **V1, état « attend »** : lancer `command claude --permission-mode default` dans une fenêtre tmux, déclencher une demande d'autorisation, puis contrôler que `status` passe à `waiting` avec `waitingFor` et que la barre et le popup l'affichent.
- **V1 bis, sessions parasites** : lancer `command claude -p "dis bonjour"` et regarder si un fichier de registre apparaît, avec quel `kind` et quel `entrypoint`. Si oui, ajouter le filtre correspondant au registre (petit correctif, avec son test).
- **V2, saut depuis le popup, fait par l'utilisateur** : le test ouvre un popup et déplace un client tmux, donc l'orchestrateur ne le fait pas sur le client réel. Il donne la marche à suivre, et l'utilisateur vérifie que `Entrée` amène sur le volet choisi, y compris avec deux clients attachés. Ce que reçoit `--client` se voit dans la ligne de commande du processus (`ps -o args`).
- **Lignes tmux** : à donner à l'utilisateur, à placer avant la ligne `run '…/tpm/tpm'`. L'orchestrateur ne les applique pas lui-même.

```tmux
set -g @tmux-dotbar-right true
set -g @tmux-dotbar-status-right '#(/home/alexis/.local/bin/claude-tower bar --dir /home/alexis/.claude) '
set -g status-right-length 120
bind a run-shell "/usr/bin/tmux display-popup -E -c '#{client_name}' -w 90 -h 60% '/home/alexis/.local/bin/claude-tower --dir /home/alexis/.claude --client #{client_name}'"
```

  - `man tmux` ne garantit le développement des formats que pour `-T` de `display-popup`, pas pour sa commande : on passe donc par `run-shell`, qui développe `#{client_name}` avant de lancer `display-popup`.

  - Le `set -g` (sans `-a`) reste idempotent au rechargement automatique de la config.
  - `--dir` explicite : le serveur tmux peut avoir hérité d'un autre `CLAUDE_CONFIG_DIR`.
  - Avec `status-justify absolute-centre`, une barre droite longue peut chevaucher la liste des fenêtres : on vérifie le rendu avec trois sessions, et on ajuste `--max-width` si besoin.
- **README.md** court, qui couvre :
  - le rôle de l'outil et son installation ;
  - les lignes tmux ;
  - les statuts reconnus (`busy`, `shell`, `idle`, `waiting`) ;
  - la limite : le format du registre est interne à Claude Code ;
  - un avertissement : la dernière activité peut afficher des commandes contenant des secrets.

## Bloc commun des briefs (lots 1 et 2)

> Dépôt : `/home/alexis/Projects/claude-tower` (dépôt git local, branche `main`, pas de remote). Ne pas committer : l'orchestrateur committe après revue. Plan : `/home/alexis/Projects/claude-tower/PLAN.md` ; exécute uniquement le lot indiqué en fin de brief.
> Ne lance aucun sous-agent ni fork : fais le travail toi-même.
> N'installe ni ne télécharge aucune dépendance. Toutes les commandes Go se lancent avec `GOPROXY=off GOFLAGS=-mod=readonly`. Ne lance jamais `go get` ni `go mod tidy`. S'il manque une dépendance, arrête-toi et signale-le.
> Ne lis et n'écris rien dans `~/.claude`, `~/.claude-perso` ni `~/.claude-shared`. Les tests utilisent uniquement des fixtures inventées.
> Commentaires de code en anglais. Ils décrivent le comportement actuel, au présent, et se comprennent en lisant le code seul : pas de référence externe (ticket, plan, PR), pas de date, pas d'historique. Pas de commentaire qui paraphrase le code : n'expliquer que ce que le code ne dit pas (contrainte, invariant, raison d'un choix). Une doc comment Go par identifiant exporté.
> Go idiomatique, bibliothèque standard autant que possible (plus `github.com/charmbracelet/x/ansi` pour la largeur d'affichage), fonctions courtes, `gofmt`.
> Le shell a des alias interactifs (`rm -i`, `ls` = eza) : utilise `command rm`, `command ls`.
> Tests : `cd /home/alexis/Projects/claude-tower && GOPROXY=off GOFLAGS=-mod=readonly go vet ./... && GOPROXY=off GOFLAGS=-mod=readonly go test ./...` doivent passer.
> Rapport final : fichiers créés ou modifiés, sortie des tests, écarts au plan et pourquoi.

Partie propre au lot : « Exécute le lot 1 » ou « Exécute le lot 2 (le lot 1 est fait ; lis le code existant avant de commencer) ».

## Revues

Pour chaque lot :
- l'orchestrateur relit le diff, lance les tests, puis `claude-tower list` sur les vraies sessions ;
- un agent Sonnet fait une revue fraîche, en ne recevant que l'objectif et l'emplacement du changement ;
- un nouvel agent fait une re-revue des seuls correctifs.

Un commit par lot sur `main`.
