package systray

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

const (
	StatusNotifierItemInterface = "org.kde.StatusNotifierItem"
	StatusNotifierItemPath      = "/StatusNotifierItem"
)

const getProperty = "org.freedesktop.DBus.Properties.Get"

// Item represents system tray item and implements [StatusNotifierItem].
//
// [StatusNotifierItem]: https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/StatusNotifierItem/
type Item struct {
	conn       *dbus.Conn
	signals    chan *dbus.Signal
	object     dbus.BusObject
	uniqueName string
	onUpdate   func()

	ID       string
	Title    string
	Tooltip  string
	Category string
	Status   string
	WindowID uint32

	IconName            string
	IconPixmap          *IconSet
	OverlayIconName     string
	OverlayIconPixmap   *IconSet
	AttentionIconName   string
	AttentionIconPixmap *IconSet
	AttentionMovieName  string

	IsMenu bool
	Menu   string
}

// NewItem returns new [Item] from its unique D-Bus name.
func NewItem(conn *dbus.Conn, uniqueName string) (*Item, error) {
	obj := conn.Object(uniqueName, StatusNotifierItemPath)

	// Check whether properties can be retrieved.
	call := obj.Call(getProperty, dbus.Flags(64), StatusNotifierItemInterface, "Title")
	if call.Err != nil {
		return nil, fmt.Errorf("failed to resolve item: %w", call.Err)
	}

	item := Item{
		conn:       conn,
		signals:    make(chan *dbus.Signal, 128),
		object:     obj,
		uniqueName: uniqueName,
		onUpdate:   func() {},
	}

	id, err := obj.GetProperty(StatusNotifierItemInterface + ".Id")
	if err == nil {
		id.Store(&item.ID)
	}

	category, err := obj.GetProperty(StatusNotifierItemInterface + ".Category")
	if err == nil {
		category.Store(&item.Category)
	}

	windowID, err := obj.GetProperty(StatusNotifierItemInterface + ".WindowId")
	if err == nil {
		windowID.Store(&item.WindowID)
	}

	isMenu, err := obj.GetProperty(StatusNotifierItemInterface + ".ItemIsMenu")
	if err == nil {
		isMenu.Store(&item.IsMenu)
	}

	menu, err := obj.GetProperty(StatusNotifierItemInterface + ".Menu")
	if err == nil {
		menu.Store(&item.Menu)
	}

	// Initialize fields that can be updated via signals.
	item.updateTitle()
	item.updateTooltip()
	item.updateStatus()
	item.updateIcon()
	item.updateOverlayIcon()
	item.updateAttentionIcon()

	// Subscribe to update signals.
	// This is required to update fields when neccessary.
	item.subscribe()

	return &item, nil
}

// NewItemFromDBusSignal returns new [Item] from D-Bus signal.
//
// It is intended to be used with signal
// org.kde.StatusNotifierWatcher.StatusNotifierItemRegistered.
func NewItemFromDBusSignal(conn *dbus.Conn, signal *dbus.Signal) (*Item, error) {
	uniqueName, err := uniqueNameFromDBusSignal(signal)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve item: %w", err)
	}

	return NewItem(conn, uniqueName)
}

// OnUpdate registers callback that runs whenever item properties are updated.
//
// The following signals with the respective update fields are specified by the
// protocol:
//
//   - NewTitle: updates Title of the item
//   - NewToolTip: updates Tooltip of the item
//   - NewStatus: updates Status of the item
//   - NewIcon: updates IconName and IconPixmap of the item.
//   - NewOverlayIcon: updates OverlayIconName and OverlayIconPixmap of the item.
//   - NewAttentionIcon: updates AttentionIconName, AttentionIconPixmap, and
//     AttentionMovieName of the item.
//
// Graphical tray hosts should redraw representation of the item when its
// OnUpdate callback is called.
func (item *Item) OnUpdate(callback func()) {
	item.onUpdate = callback
}

// ContextMenu asks the status notifier item to show a context menu.
//
// This is typically a consequence of user input, such as mouse right click
// over the graphical representation of the item.
//
// The x and y parameters are in screen coordinates and is to be considered a
// hint to the item about where to show the context menu.
func (item *Item) ContextMenu(x, y int) error {
	return item.object.Call(
		StatusNotifierItemInterface+".ContextMenu",
		dbus.Flags(64),
		x, y,
	).Err
}

// Activate asks the status notifier item for activation. The application will
// perform any task is considered appropriate as an activation request.
//
// This is typically a consequence of user input, such as mouse left click over
// the graphical representation of the item.
//
// The x and y parameters are in screen coordinates and is to be considered a
// hint to the item where to show eventual windows (if any).
func (item *Item) Activate(x, y int) error {
	return item.object.Call(
		StatusNotifierItemInterface+".Activate",
		dbus.Flags(64),
		x, y,
	).Err
}

// SecondaryActivate is to be considered a secondary and less important form of
// activation compared to Activate. The application will perform any task is
// considered appropriate as an activation request.
//
// This is typically a consequence of user input, such as mouse middle click
// over the graphical representation of the item.
//
// The x and y parameters are in screen coordinates and is to be considered a
// hint to the item where to show eventual windows (if any).
func (item *Item) SecondaryActivate(x, y int) error {
	return item.object.Call(
		StatusNotifierItemInterface+".SecondaryActivate",
		dbus.Flags(64),
		x, y,
	).Err
}

// Scroll emits a scroll event on the status notifier item.
//
// This is caused from input such as mouse wheel over the graphical
// representation of the item.
//
// The delta parameter represent the amount of scroll. The orientation
// parameter represent orientation of the scroll request and its valid values
// are "horizontal" and "vertical".
func (item *Item) Scroll(delta int, orientation string) error {
	return item.object.Call(
		StatusNotifierItemInterface+".Scroll",
		dbus.Flags(64),
		delta, orientation,
	).Err
}

// close removes signal handlers associated with this item.
//
// This method must be called when item is being unregistered from the system tray.
func (item *Item) close() {
	item.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewTitle"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewToolTip"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewStatus"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewIcon"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewOverlayIcon"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewAttentionIcon"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.RemoveSignal(item.signals)
	close(item.signals)
}

func (item *Item) subscribe() {
	item.conn.AddMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewTitle"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.AddMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewToolTip"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.AddMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewStatus"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.AddMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewIcon"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.AddMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewOverlayIcon"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.AddMatchSignal(
		dbus.WithMatchInterface(StatusNotifierItemInterface),
		dbus.WithMatchMember("NewAttentionIcon"),
		dbus.WithMatchSender(item.uniqueName),
	)

	item.conn.Signal(item.signals)

	go func() {
		for signal := range item.signals {
			if signal.Sender != item.uniqueName {
				continue
			}

			item.handleSignal(signal)
			item.onUpdate()
		}
	}()
}

func (item *Item) handleSignal(signal *dbus.Signal) {
	switch signal.Name {
	case StatusNotifierItemInterface + ".NewTitle":
		item.updateTitle()
	case StatusNotifierItemInterface + ".NewToolTip":
		item.updateTooltip()
	case StatusNotifierItemInterface + ".NewStatus":
		item.updateStatus()
	case StatusNotifierItemInterface + ".NewIcon":
		item.updateIcon()
	case StatusNotifierItemInterface + ".NewOverlayIcon":
		item.updateOverlayIcon()
	case StatusNotifierItemInterface + ".NewAttentionIcon":
		item.updateAttentionIcon()
	}
}

// updateTitle initializes or updates Title of the item.
func (item *Item) updateTitle() {
	title, err := item.object.GetProperty(StatusNotifierItemInterface + ".Title")
	if err == nil {
		title.Store(&item.Title)
	}
}

// updateTooltip initializes or updates Tooltip of the item.
func (item *Item) updateTooltip() {
	tooltip, err := item.object.GetProperty(StatusNotifierItemInterface + ".ToolTip")
	if err == nil {
		// Format of tooltip is as follows
		//
		//  [<icon-name>, <icon>, <tooltip>, <description>]
		//
		// We are interested in the 3rd item, as it is a text representation of the
		// tooltip.
		value := tooltip.Value().([]any)

		if len(value) >= 3 {
			tooltipStr, ok := value[2].(string)
			if ok {
				item.Tooltip = tooltipStr
			}
		}
	}
}

// updateStatus initializes or updates Status of the item.
func (item *Item) updateStatus() {
	status, err := item.object.GetProperty(StatusNotifierItemInterface + ".Status")
	if err == nil {
		status.Store(&item.Status)
	}
}

// updateIcon initializes or updates IconName and IconPixmap of the item.
func (item *Item) updateIcon() {
	iconName, err := item.object.GetProperty(StatusNotifierItemInterface + ".IconName")
	if err == nil {
		iconName.Store(&item.IconName)
	}

	iconPixmap, err := item.object.GetProperty(StatusNotifierItemInterface + ".IconPixmap")
	if err == nil {
		iconset, err := NewIconSetFromDBusProperty(iconPixmap.Value())
		if err == nil {
			item.IconPixmap = iconset
		}
	}
}

// updateOverlayIcon initializes or updates OverlayIconName and
// OverlayIconPixmap of the item.
func (item *Item) updateOverlayIcon() {
	overlayIconName, err := item.object.GetProperty(StatusNotifierItemInterface + ".OverlayIconName")
	if err == nil {
		overlayIconName.Store(&item.OverlayIconName)
	}

	overlayIconPixmap, err := item.object.GetProperty(StatusNotifierItemInterface + ".OverlayIconPixmap")
	if err == nil {
		iconset, err := NewIconSetFromDBusProperty(overlayIconPixmap.Value())
		if err == nil {
			item.OverlayIconPixmap = iconset
		}
	}
}

// updateAttentionIcon initializes or updates AttentionIconName,
// AttentionIconPixmap, and AttentionMovieName of the item.
func (item *Item) updateAttentionIcon() {
	attentionIconName, err := item.object.GetProperty(StatusNotifierItemInterface + ".AttentionIconName")
	if err == nil {
		attentionIconName.Store(&item.AttentionIconName)
	}

	attentionIconPixmap, err := item.object.GetProperty(StatusNotifierItemInterface + ".AttentionIconPixmap")
	if err == nil {
		iconset, err := NewIconSetFromDBusProperty(attentionIconPixmap.Value())
		if err == nil {
			item.AttentionIconPixmap = iconset
		}
	}

	attentionMovieName, err := item.object.GetProperty(StatusNotifierItemInterface + ".AttentionMovieName")
	if err == nil {
		attentionMovieName.Store(&item.AttentionMovieName)
	}
}

// uniqueNameFromDBusSignal retrieves unique name of the StatusNotifierItem
// service from D-Bus signal.
func uniqueNameFromDBusSignal(signal *dbus.Signal) (string, error) {
	if len(signal.Body) < 1 {
		return "", fmt.Errorf("signal body is empty")
	}

	itemName, ok := signal.Body[0].(string)
	if !ok {
		return "", fmt.Errorf("invalid format of signal body")
	}

	// Format of itemName is "<uniqueName>/StatusNotifierItem",
	// e.g. :1.185/StatusNotifierItem
	uniqueName, _, ok := strings.Cut(itemName, "/")
	if !ok {
		return "", fmt.Errorf("invalid format of item name")
	}

	return uniqueName, nil
}
