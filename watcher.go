package systray

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

const (
	StatusNotifierWatcherInterface = "org.kde.StatusNotifierWatcher"
	StatusNotifierWatcherPath      = "/StatusNotifierWatcher"
)

type Watcher struct {
	closed  bool
	conn    *dbus.Conn
	mu      sync.Mutex
	signals chan *dbus.Signal
	hosts   []string
	items   []string
}

func NewWatcher(conn *dbus.Conn) *Watcher {
	return &Watcher{
		closed:  false,
		conn:    conn,
		signals: make(chan *dbus.Signal, 64),
	}
}

func (w *Watcher) Listen() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("listen: watcher is closed")
	}

	reply, err := w.conn.RequestName(StatusNotifierWatcherInterface, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("listen: failed to request name %s: %w", StatusNotifierWatcherInterface, err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("listen: name %s already taken", StatusNotifierWatcherInterface)
	}

	if err := w.conn.Export(w, StatusNotifierWatcherPath, StatusNotifierWatcherInterface); err != nil {
		return fmt.Errorf("listen: failed to export %s: %w", StatusNotifierWatcherInterface, err)
	}

	w.exportProperties()
	go w.subscribe()

	return nil
}

func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	_, err := w.conn.ReleaseName(StatusNotifierWatcherInterface)
	if err != nil {
		return err
	}

	for _, host := range w.hosts {
		w.conn.RemoveMatchSignal(
			dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchSender("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, host),
		)
	}

	for _, item := range w.items {
		// Since items are stored as
		//
		//  <uniqueName>/<path>
		//
		// and signals match against uniqueName, we need to extract uniqueName.
		uniqueName, _, err := uniqueNameAndPathFromItemName(item)
		if err != nil {
			continue
		}

		w.conn.RemoveMatchSignal(
			dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchSender("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, uniqueName),
		)
	}

	w.conn.RemoveSignal(w.signals)
	close(w.signals)

	w.closed = true

	return nil
}

func (w *Watcher) RegisterStatusNotifierItem(name string, sender dbus.Sender) *dbus.Error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if slices.Contains(w.items, name) {
		return nil
	}

	identifier := string(name) + StatusNotifierItemPath

	if strings.HasPrefix(name, "/") {
		identifier = string(sender) + name
	}

	w.items = append(w.items, identifier)

	// Watch for name owner changes.
	// Whenever name disappears, D-Bus will send NameOwnerChanged signal with
	// empty NewOwner argument. In this case, item should be unregistered.
	w.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, string(sender)),
	)

	w.conn.Emit(StatusNotifierWatcherPath, StatusNotifierWatcherInterface+".StatusNotifierItemRegistered", identifier)
	w.exportProperties()

	return nil
}

func (w *Watcher) RegisterStatusNotifierHost(name string) *dbus.Error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if slices.Contains(w.hosts, name) {
		return nil
	}

	w.hosts = append(w.hosts, name)

	w.conn.Emit(StatusNotifierWatcherPath, StatusNotifierWatcherInterface+".StatusNotifierHostRegistered", name)
	w.exportProperties()

	// Watch for name owner changes.
	// Whenever name disappears, D-Bus will send NameOwnerChanged signal with
	// empty NewOwner argument. In this case, item should be unregistered.
	w.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, name),
	)

	return nil
}

func (w *Watcher) subscribe() {
	w.conn.Signal(w.signals)

	go func() {
		for signal := range w.signals {
			if signal.Name != "org.freedesktop.DBus.NameOwnerChanged" {
				continue
			}

			if len(signal.Body) < 3 {
				continue
			}

			name, ok := signal.Body[0].(string)
			if !ok {
				continue
			}

			newOwner, ok := signal.Body[2].(string)
			if !ok {
				continue
			}

			if newOwner == "" {
				w.tryUnregisterHost(name)
				w.tryUnregisterItem(name)
			}
		}
	}()
}

func (w *Watcher) tryUnregisterHost(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	identifier := ""
	identifierIndex := -1

	for idx, host := range w.hosts {
		if host == name {
			identifier = host
			identifierIndex = idx
		}
	}

	if identifier == "" {
		return
	}

	w.conn.RemoveMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, name),
	)

	w.hosts = append(w.hosts[:identifierIndex], w.hosts[identifierIndex+1:]...)
	w.exportProperties()
}

func (w *Watcher) tryUnregisterItem(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	identifier := ""
	identifierIndex := -1

	for idx, item := range w.items {
		if strings.HasPrefix(item, name) {
			identifier = item
			identifierIndex = idx
			break
		}
	}

	if identifier == "" {
		return
	}

	w.conn.RemoveMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, name),
	)

	w.items = append(w.items[:identifierIndex], w.items[identifierIndex+1:]...)
	w.conn.Emit(StatusNotifierWatcherPath, StatusNotifierWatcherInterface+".StatusNotifierItemUnregistered", identifier)
	w.exportProperties()
}

func (w *Watcher) exportProperties() {
	prop.Export(w.conn, StatusNotifierWatcherPath, prop.Map{
		StatusNotifierWatcherInterface: map[string]*prop.Prop{
			"RegisteredStatusNotifierItems": {
				Value:    w.items,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"IsStatusNotifierHostRegistered": {
				Value:    len(w.hosts) > 0,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"ProtocolVersion": {
				Value:    1,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
		},
	})
}
