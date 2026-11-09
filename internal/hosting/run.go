package hosting

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Run is `claude-tower run`: it replaces the current process with Claude Code
// given args, which are passed through untouched. Inside tmux, claude runs in
// place; elsewhere it runs in the private server and the process attaches to
// it, so that closing the terminal only detaches.
func (h *Hosting) Run(args []string) error {
	ctx := context.Background()
	if envHas(h.Env, "TMUX") {
		argv, err := h.resolveClaude(ctx, h.Env)
		if err != nil {
			return err
		}
		return h.execFn()(argv[0], append(argv, args...), h.Env)
	}
	cwd, err := h.getwd()
	if err != nil {
		return err
	}
	callerEnv := WithUTF8Locale(h.Env)
	pane, err := h.NewSession(ctx, cwd, args, SessionEnv(callerEnv, ""))
	if err != nil {
		return err
	}
	srv, err := h.server()
	if err != nil {
		return err
	}
	bin := srv.Bin
	if !strings.Contains(bin, "/") {
		if bin, err = exec.LookPath(bin); err != nil {
			return err
		}
	}
	return h.execFn()(bin, []string{bin, "-S", srv.Socket, "attach", "-t", pane.SessionID}, callerEnv)
}

func (h *Hosting) getwd() (string, error) {
	if h.Cwd != nil {
		return h.Cwd()
	}
	return os.Getwd()
}

// ExecEnvArgs splits the arguments of `claude-tower exec-env` into the
// environment file and the command to run.
func ExecEnvArgs(args []string) (file string, command []string, err error) {
	if len(args) < 3 || args[1] != "--" {
		return "", nil, errors.New("usage: claude-tower exec-env <fichier> -- <commande> [args…]")
	}
	return args[0], args[2:], nil
}
