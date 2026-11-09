# claude-tower

Suit d'un coup d'œil les sessions Claude Code en cours sur la machine, et permet d'y sauter directement dans tmux.

- `claude-tower bar` : segment pour la barre d'état tmux.
- `claude-tower` : popup listant les sessions ; `Entrée` amène sur le volet de la session choisie.
- `claude-tower list` : la même liste en texte brut, pour déboguer.

L'outil est en lecture seule : pas de hook, pas de serveur, pas de base de données. Il lit le registre des sessions de Claude Code (`~/.claude/sessions/*.json`) et la fin des transcripts (`~/.claude/projects/*/<session>.jsonl`).

## États

| Barre | Popup | Signification |
| --- | --- | --- |
| `!` rouge | attend (motif) | la session attend une réponse (autorisation, question…) |
| `●` jaune | travaille / shell | un tour est en cours |
| `◐` cyan | fini · fond | le tour est fini, mais des shells ou agents tournent en arrière-plan |
| `○` gris | fini | la session attend un nouveau message |
| `?` gris | (statut brut) | statut du registre non reconnu |

La barre trie les sessions par nom. Quand elle dépasse `--max-width` (100 colonnes par défaut), les sessions finies (y compris en fond et d'état inconnu) sont d'abord regroupées en un compteur gris `○ n`, puis celles qui travaillent en `● n` ; les sessions qui attendent restent toujours affichées.

Le popup trie par urgence. Touches : `↑`/`k`, `↓`/`j`, `1`…`9` (sélection), `Entrée` (aller), `q`/`Échap`/`Ctrl-C` (fermer).

## Installation

```sh
make test
make install    # construit ~/.local/bin/claude-tower
```

Les dépendances (Bubble Tea, lipgloss) doivent déjà être dans le cache de modules Go : les commandes tournent avec `GOPROXY=off`.

## tmux

À placer dans `~/.config/tmux/tmux.conf`, avant la ligne `run '…/tpm/tpm'` (le segment passe par tmux-dotbar, qui réécrit `status-right` au chargement) :

```tmux
set -g @tmux-dotbar-right true
set -g @tmux-dotbar-status-right '#(/home/alexis/.local/bin/claude-tower bar --dir /home/alexis/.claude) '
set -g status-right-length 120
bind a run-shell "/usr/bin/tmux display-popup -E -c '#{client_name}' -w 90 -h 60% '/home/alexis/.local/bin/claude-tower --dir /home/alexis/.claude --client #{client_name}'"
```

- `--dir` est explicite, car le serveur tmux peut avoir hérité d'un autre `CLAUDE_CONFIG_DIR`.
- `run-shell` développe `#{client_name}` avant d'ouvrir le popup ; le saut vise ainsi le bon client quand plusieurs sont attachés.

## Limites

- Le registre des sessions est un format interne de Claude Code, sans garantie de stabilité : une mise à jour peut casser l'outil. Tout ce qui en dépend est dans `internal/registry`.
- Le saut vérifie que le volet tmux contient bien le processus de la session ; une session lancée dans un autre serveur tmux affiche « volet introuvable sur ce serveur tmux ».
- La dernière activité affiche le début des commandes lancées par Claude : elle peut contenir des secrets visibles à l'écran.
