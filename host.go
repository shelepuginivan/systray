package systray

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// Host implements [StatusNotifierHost]. It keeps track of StatusNotifierItem
// instances via [StatusNotifierWatcher].
//
// [StatusNotifierHost]: https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/StatusNotifierHost/
// [StatusNotifierWatcher]: https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/StatusNotifierWatcher/
type Host struct {
	name           string
	closed         bool
	conn           *dbus.Conn
	items          map[string]*Item
	signals        chan *dbus.Signal
	mu             sync.RWMutex
	onRegistered   func(item *Item)
	onUnregistered func(item *Item)
}

// NewHost returns a new [Host].
//
// Parameter id is used as a unique identifier for host name, such as PID.
func NewHost(conn *dbus.Conn, id any) *Host {
	h := &Host{
		name:           fmt.Sprintf("org.kde.StatusNotifierHost-%v", id),
		closed:         false,
		conn:           conn,
		items:          make(map[string]*Item),
		signals:        make(chan *dbus.Signal, 64),
		onRegistered:   func(*Item) {},
		onUnregistered: func(*Item) {},
	}

	return h
}

// Name returns name of the host service.
func (h *Host) Name() string {
	return h.name
}

// Listen requests name of the host on D-Bus, queries items that are already
// registered, and subscribes to signals.
//
// This method should be called after [Host.OnRegistered] and
// [Host.OnUnregistered] callbacks were set.
//
// If Listen is called after [Host.Close], an error is returned.
func (h *Host) Listen() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return fmt.Errorf("listen: host is closed")
	}

	reply, err := h.conn.RequestName(h.name, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("listen: failed to request name %s: %w", h.name, err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("listen: name %s already taken", h.name)
	}

	// Register host in the watcher.
	call := h.conn.Object(
		StatusNotifierWatcherInterface,
		StatusNotifierWatcherPath,
	).Call("RegisterStatusNotifierHost", 0, h.name)
	if call.Err != nil {
		return fmt.Errorf("listen: failed to register host: %w", call.Err)
	}

	if err := h.subscribe(); err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	h.getInitialItems()

	return nil
}

// Close releases name of the host from D-Bus and unsubscribes from signals.
//
// Host cannot be reused after Close was called.
func (h *Host) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := h.conn.ReleaseName(h.name)
	if err != nil {
		return err
	}

	if err := h.conn.RemoveMatchSignal(
		dbus.WithMatchInterface("org.kde.StatusNotifierWatcher"),
		dbus.WithMatchMember("StatusNotifierItemRegistered"),
	); err != nil {
		return err
	}

	if err := h.conn.RemoveMatchSignal(
		dbus.WithMatchInterface("org.kde.StatusNotifierWatcher"),
		dbus.WithMatchMember("StatusNotifierItemUnregistered"),
	); err != nil {
		return err
	}

	h.conn.RemoveSignal(h.signals)
	close(h.signals)

	// Close all items to unregister signals from the session bus.
	for _, item := range h.items {
		item.close()
	}

	h.onRegistered = nil
	h.onUnregistered = nil
	h.closed = true

	return nil
}

// Items returns currently registered items.
func (h *Host) Items() []*Item {
	h.mu.RLock()
	defer h.mu.RUnlock()

	items := make([]*Item, len(h.items))
	idx := 0

	for _, item := range h.items {
		items[idx] = item
		idx++
	}

	return items
}

// OnRegistered sets callback that runs whenever a new item is registered.
//
// Graphical tray hosts should draw item representation when OnRegistered
// callback is called.
func (h *Host) OnRegistered(callback func(*Item)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.onRegistered = callback
}

// OnUnregistered sets callback that runs whenever a new item is unregistered.
//
// Graphical tray hosts should destroy item representation when OnUnregistered
// callback is called.
func (h *Host) OnUnregistered(callback func(*Item)) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.onUnregistered = callback
}

// getInitialItems retrieves items that are already registered.
func (h *Host) getInitialItems() {
	watcherObj := h.conn.Object(StatusNotifierWatcherInterface, StatusNotifierWatcherPath)

	property, err := watcherObj.GetProperty(StatusNotifierWatcherInterface + ".RegisteredStatusNotifierItems")
	if err != nil {
		return
	}

	registeredItems, ok := property.Value().([]string)
	if !ok {
		return
	}

	for _, itemName := range registeredItems {
		uniqueName, objectPath, err := uniqueNameAndPathFromItemName(itemName)
		if err != nil {
			continue
		}

		if h.isRegistered(uniqueName) {
			continue
		}

		item, err := NewItemWithObjectPath(h.conn, uniqueName, objectPath)
		if err != nil {
			continue
		}

		h.items[uniqueName] = item
		h.onRegistered(item)
	}
}

// subscribe subscribes to signals
//   - org.kde.StatusNotifierWatcher.StatusNotifierItemRegistered
//   - org.kde.StatusNotifierWatcher.StatusNotifierItemUnregistered
func (h *Host) subscribe() error {
	if err := h.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.kde.StatusNotifierWatcher"),
		dbus.WithMatchMember("StatusNotifierItemRegistered"),
	); err != nil {
		return err
	}

	if err := h.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.kde.StatusNotifierWatcher"),
		dbus.WithMatchMember("StatusNotifierItemUnregistered"),
	); err != nil {
		return err
	}

	h.conn.Signal(h.signals)

	go func() {
		for signal := range h.signals {
			switch signal.Name {
			case StatusNotifierWatcherInterface + ".StatusNotifierItemRegistered":
				h.handleRegisteredSignal(signal)
			case StatusNotifierWatcherInterface + ".StatusNotifierItemUnregistered":
				h.handleUnregisteredSignal(signal)
			}
		}
	}()

	return nil
}

// isRegistered reports whether name is already registered in the host.
func (h *Host) isRegistered(uniqueName string) bool {
	_, exists := h.items[uniqueName]
	return exists
}

// handleRegisteredSignal handles the
// org.kde.StatusNotifierWatcher.StatusNotifierItemRegistered signal
func (h *Host) handleRegisteredSignal(signal *dbus.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uniqueName, objectPath, err := uniqueNameAndPathFromDBusSignal(signal)
	if err != nil {
		return
	}

	if h.isRegistered(uniqueName) {
		return
	}

	item, err := NewItemWithObjectPath(h.conn, uniqueName, objectPath)
	if err != nil {
		return
	}

	h.items[item.uniqueName] = item
	h.onRegistered(item)
}

// handleUnregisteredSignal handles the
// org.kde.StatusNotifierWatcher.StatusNotifierItemUnregistered signal.
func (h *Host) handleUnregisteredSignal(signal *dbus.Signal) {
	h.mu.Lock()
	defer h.mu.Unlock()

	uniqueName, _, err := uniqueNameAndPathFromDBusSignal(signal)
	if err != nil {
		return
	}

	item, exists := h.items[uniqueName]
	if !exists {
		return
	}

	h.onUnregistered(item)
	item.close()
	delete(h.items, uniqueName)
}
