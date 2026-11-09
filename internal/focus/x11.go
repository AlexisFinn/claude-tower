package focus

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// sourceIndication marks the request as coming from a pager or taskbar, which
// window managers honor over an application's own request.
const sourceIndication = 2

// window is a top-level window with the process that owns it.
type window struct {
	id  xproto.Window
	pid int
}

func raiseX11(pids []int) error {
	conn, err := xgb.NewConn()
	if err != nil {
		return fmt.Errorf("connexion à X11 : %w", err)
	}
	defer conn.Close()
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	activeAtom, err := atom(conn, "_NET_ACTIVE_WINDOW")
	if err != nil {
		return err
	}
	windows, err := clientWindows(conn, root)
	if err != nil {
		return err
	}
	w, ok := firstWindow(pids, windows)
	if !ok {
		return errors.New("aucune fenêtre trouvée pour cette session")
	}
	ev := activeWindowMessage(w, activeAtom)
	const mask = xproto.EventMaskSubstructureRedirect | xproto.EventMaskSubstructureNotify
	if err := xproto.SendEventChecked(conn, false, root, mask, string(ev.Bytes())).Check(); err != nil {
		return fmt.Errorf("_NET_ACTIVE_WINDOW : %w", err)
	}
	return nil
}

func atom(conn *xgb.Conn, name string) (xproto.Atom, error) {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil {
		return 0, fmt.Errorf("atome %s : %w", name, err)
	}
	return reply.Atom, nil
}

// clientWindows lists the managed windows, in the order of _NET_CLIENT_LIST,
// with the PID each declares in _NET_WM_PID. Windows without one are skipped.
func clientWindows(conn *xgb.Conn, root xproto.Window) ([]window, error) {
	listAtom, err := atom(conn, "_NET_CLIENT_LIST")
	if err != nil {
		return nil, err
	}
	pidAtom, err := atom(conn, "_NET_WM_PID")
	if err != nil {
		return nil, err
	}
	reply, err := xproto.GetProperty(conn, false, root, listAtom, xproto.AtomWindow, 0, 1<<16).Reply()
	if err != nil {
		return nil, fmt.Errorf("_NET_CLIENT_LIST : %w", err)
	}
	var out []window
	for _, id := range parseCardinals(reply.Value) {
		prop, err := xproto.GetProperty(conn, false, xproto.Window(id), pidAtom, xproto.AtomCardinal, 0, 1).Reply()
		if err != nil {
			continue
		}
		if pids := parseCardinals(prop.Value); len(pids) == 1 {
			out = append(out, window{xproto.Window(id), int(pids[0])})
		}
	}
	return out, nil
}

// parseCardinals decodes a property value made of 32-bit little-endian
// words, ignoring a trailing partial word.
func parseCardinals(value []byte) []uint32 {
	out := make([]uint32, 0, len(value)/4)
	for i := 0; i+4 <= len(value); i += 4 {
		out = append(out, binary.LittleEndian.Uint32(value[i:]))
	}
	return out
}

// firstWindow returns the first window of the process that comes first in
// pids. A terminal with several windows yields the first one listed.
func firstWindow(pids []int, windows []window) (xproto.Window, bool) {
	for _, pid := range pids {
		for _, w := range windows {
			if w.pid == pid {
				return w.id, true
			}
		}
	}
	return 0, false
}

// activeWindowMessage builds the _NET_ACTIVE_WINDOW request for win.
func activeWindowMessage(win xproto.Window, activeAtom xproto.Atom) xproto.ClientMessageEvent {
	return xproto.ClientMessageEvent{
		Format: 32,
		Window: win,
		Type:   activeAtom,
		Data:   xproto.ClientMessageDataUnionData32New([]uint32{sourceIndication, 0, 0, 0, 0}),
	}
}
