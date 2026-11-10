// Command claude-tower shows the Claude Code sessions running on this machine.
// Without a subcommand it opens the desktop application.
package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"time"

	"claude-tower/internal/app"
	"claude-tower/internal/cli"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	if cli.IsCLI(os.Args[1:]) {
		os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
	}
	if err := runApp(); err != nil {
		fmt.Fprintf(os.Stderr, "claude-tower: %v\n", err)
		os.Exit(1)
	}
}

// window gives the application the events and dialogs of the Wails runtime,
// which need the context received at startup. The context is written by
// setContext and only read after ready is closed.
type window struct {
	ctx   context.Context
	ready chan struct{}
}

const windowWait = 30 * time.Second

func newWindow() *window {
	return &window{ready: make(chan struct{})}
}

func (w *window) setContext(ctx context.Context) {
	w.ctx = ctx
	close(w.ready)
}

// context returns the context of the window, or an error when it is not
// available after windowWait.
func (w *window) context() (context.Context, error) {
	select {
	case <-w.ready:
		return w.ctx, nil
	case <-time.After(windowWait):
		return nil, errors.New("démarrage trop long")
	}
}

func (w *window) Emit(name string, data any) {
	if ctx, err := w.context(); err == nil {
		runtime.EventsEmit(ctx, name, data)
	}
}

func (w *window) ChooseDirectory() (string, error) {
	ctx, err := w.context()
	if err != nil {
		return "", err
	}
	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{Title: "Dossier de la nouvelle session"})
}

func (w *window) ChooseFiles() ([]string, error) {
	ctx, err := w.context()
	if err != nil {
		return nil, err
	}
	return runtime.OpenMultipleFilesDialog(ctx, runtime.OpenDialogOptions{Title: "Fichiers à joindre"})
}

func runApp() error {
	w := newWindow()
	a := app.New(w, w, os.Environ())
	return wails.Run(&options.App{
		Title:            "Claude Tower",
		Width:            1400,
		Height:           900,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: &options.RGBA{R: 11, G: 14, B: 19, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		Linux:            &linux.Options{Icon: icon, ProgramName: "claude-tower"},
		Mac:              &mac.Options{About: &mac.AboutInfo{Title: "Claude Tower", Icon: icon}},
		OnStartup: func(ctx context.Context) {
			w.setContext(ctx)
			app.Startup(a)
		},
		OnShutdown: func(context.Context) { app.Shutdown(a) },
		Bind:       []any{a},
	})
}
