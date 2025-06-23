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
	object     dbus.BusObject
	uniqueName string

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
		object:     obj,
		uniqueName: uniqueName,
	}

	id, err := obj.GetProperty(StatusNotifierItemInterface + ".Id")
	if err == nil {
		id.Store(&item.ID)
	}

	title, err := obj.GetProperty(StatusNotifierItemInterface + ".Title")
	if err == nil {
		title.Store(&item.Title)
	}

	tooltip, err := obj.GetProperty(StatusNotifierItemInterface + ".ToolTip")
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

	category, err := obj.GetProperty(StatusNotifierItemInterface + ".Category")
	if err == nil {
		category.Store(&item.Category)
	}

	status, err := obj.GetProperty(StatusNotifierItemInterface + ".Status")
	if err == nil {
		status.Store(&item.Status)
	}

	windowID, err := obj.GetProperty(StatusNotifierItemInterface + ".WindowId")
	if err == nil {
		windowID.Store(&item.WindowID)
	}

	iconName, err := obj.GetProperty(StatusNotifierItemInterface + ".IconName")
	if err == nil {
		iconName.Store(&item.IconName)
	}

	iconPixmap, err := obj.GetProperty(StatusNotifierItemInterface + ".IconPixmap")
	if err == nil {
		iconset, err := NewIconSetFromDBusProperty(iconPixmap.Value())
		if err == nil {
			item.IconPixmap = iconset
		}
	}

	overlayIconName, err := obj.GetProperty(StatusNotifierItemInterface + ".OverlayIconName")
	if err == nil {
		overlayIconName.Store(&item.OverlayIconName)
	}

	overlayIconPixmap, err := obj.GetProperty(StatusNotifierItemInterface + ".OverlayIconPixmap")
	if err == nil {
		iconset, err := NewIconSetFromDBusProperty(overlayIconPixmap.Value())
		if err == nil {
			item.OverlayIconPixmap = iconset
		}
	}

	attentionIconName, err := obj.GetProperty(StatusNotifierItemInterface + ".AttentionIconName")
	if err == nil {
		attentionIconName.Store(&item.AttentionIconName)
	}

	attentionIconPixmap, err := obj.GetProperty(StatusNotifierItemInterface + ".AttentionIconPixmap")
	if err == nil {
		iconset, err := NewIconSetFromDBusProperty(attentionIconPixmap.Value())
		if err == nil {
			item.AttentionIconPixmap = iconset
		}
	}

	attentionMovieName, err := obj.GetProperty(StatusNotifierItemInterface + ".AttentionMovieName")
	if err == nil {
		attentionMovieName.Store(&item.AttentionMovieName)
	}

	isMenu, err := obj.GetProperty(StatusNotifierItemInterface + ".ItemIsMenu")
	if err == nil {
		isMenu.Store(&item.IsMenu)
	}

	menu, err := obj.GetProperty(StatusNotifierItemInterface + ".Menu")
	if err == nil {
		menu.Store(&item.Menu)
	}

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
