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

type ItemCategory string

// StatusNotifierItem categories.
const (
	// The item describes the status of a generic application, for instance the
	// current state of a media player.
	ItemCategoryApplicationStatus ItemCategory = "ApplicationStatus"

	// The item describes the status of communication oriented applications, like
	// an instant messenger or an email client.
	ItemCategoryCommunications ItemCategory = "Communications"

	// The item describes services of the system not seen as a stand alone
	// application by the user, such as an indicator for the activity of a disk
	// indexing service.
	ItemCategorySystemServices ItemCategory = "SystemServices"

	// The item describes the state and control of a particular hardware, such as
	// an indicator of the battery charge or sound card volume control.
	ItemCategoryHardware ItemCategory = "Hardware"
)

type ItemStatus string

// StatusNotifierItem statuses.
const (
	// The item doesn't convey important information to the user, it can be
	// considered an "idle" status and is likely that visualizations will choose
	// to hide it.
	ItemStatusPassive ItemStatus = "Passive"

	// The item is active, is more important that the item will be shown in some
	// way to the user.
	ItemStatusActive ItemStatus = "Active"

	// The item carries really important information for the user, such as battery
	// charge running out and is wants to incentive the direct user intervention.
	// Visualizations should emphasize in some way the items with NeedsAttention
	// status.
	ItemStatusNeedsAttention ItemStatus = "NeedsAttention"
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

	// Unique identifier for the application, such as the application name.
	ID string

	// Name that describes the application, can be more descriptive than ID.
	Title string

	// Extra information that can be visualized by a tooltip.
	Tooltip string

	// Category of the item.
	Category ItemCategory

	// Status of the item or of the associated application.
	Status ItemStatus

	// Windowing-system dependent identifier.
	WindowID uint32

	// Icon that is used to visualize the item.
	//
	// IconName is a [Freedesktop-compliant] icon name. Visualizations should
	// prefer this field over IconPixmap if both are available.
	//
	// [Freedesktop-compliant]: https://specifications.freedesktop.org/icon-naming-spec/latest/
	IconName string

	// Icon that is used to visualize the item.
	//
	// IconPixmap is a binary representation of the icon.
	IconPixmap *IconSet

	// Icon that indicates extra information and can be used by the visualization
	// as an overlay for the main icon.
	//
	// OverlayIconName is a [Freedesktop-compliant] overlay icon name.
	// Visualizations should prefer this field over OverlayIconPixmap if both are
	// available.
	//
	// [Freedesktop-compliant]: https://specifications.freedesktop.org/icon-naming-spec/latest/
	OverlayIconName string

	// Icon that indicates extra information and can be used by the visualization
	// as an overlay for the main icon.
	//
	// OverlayIconPixmap is a binary representation of the overlay icon.
	OverlayIconPixmap *IconSet

	// Icon that can be used by the visualization to indicate that the item needs
	// attention.
	//
	// AttentionIconName is a [Freedesktop-compliant] attention icon name.
	// Visualizations should prefer this field over AttentionIconPixmap if both
	// are available.
	AttentionIconName string

	// Icon that can be used by the visualization to indicate that the item needs
	// attention.
	//
	// AttentionIconPixmap is a binary representation of the attention icon.
	AttentionIconPixmap *IconSet

	// Animation that can be used by the visualizations, either a
	// [Freedesktop-compliant] icon or a full path.
	//
	// The visualization can choose between this field and AttentionIconPixmap to
	// indicate that item needs attention.
	//
	// [Freedesktop-compliant]: https://specifications.freedesktop.org/icon-naming-spec/latest/
	AttentionMovieName string

	// Whether the item only supports context menu. Visualizations should prefer
	// to show the [Item.Menu] or calling [Item.ContextMenu] instead of
	// [Item.Activate].
	IsMenu bool

	// D-Bus path to an object which implements the com.canonical.dbusmenu
	// interface.
	MenuPath string
}

// NewItem returns new [Item] from its unique D-Bus name.
func NewItem(conn *dbus.Conn, uniqueName string) (*Item, error) {
	return NewItemWithObjectPath(conn, uniqueName, StatusNotifierItemPath)
}

// NewItemWithObjectPath returns new [Item] from its unique D-Bus name and
// allows to specify path of the D-Bus object.
func NewItemWithObjectPath(conn *dbus.Conn, uniqueName string, objectPath string) (*Item, error) {
	obj := conn.Object(uniqueName, dbus.ObjectPath(objectPath))

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
		switch category.String() {
		case "Communications":
			item.Category = ItemCategoryCommunications
		case "SystemServices":
			item.Category = ItemCategorySystemServices
		case "Hardware":
			item.Category = ItemCategoryHardware
		default:
			item.Category = ItemCategoryApplicationStatus
		}
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
		menu.Store(&item.MenuPath)
	}

	// Initialize fields that can be updated via signals.
	item.updateTitle()
	item.updateTooltip()
	item.updateStatus()
	item.updateIcon()
	item.updateOverlayIcon()
	item.updateAttentionIcon()

	// Subscribe to update signals.
	// This is required to update fields when necessary.
	item.subscribe()

	return &item, nil
}

// NewItemFromDBusSignal returns new [Item] from D-Bus signal.
//
// It is intended to be used with signal
// org.kde.StatusNotifierWatcher.StatusNotifierItemRegistered.
func NewItemFromDBusSignal(conn *dbus.Conn, signal *dbus.Signal) (*Item, error) {
	uniqueName, objectPath, err := uniqueNameAndPathFromDBusSignal(signal)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve item: %w", err)
	}

	return NewItemWithObjectPath(conn, uniqueName, objectPath)
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

// Menu returns [Menu] object associated with item.
func (item *Item) Menu() (*Menu, error) {
	return NewMenu(item.conn, item.uniqueName, item.MenuPath)
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
		switch status.String() {
		case "Passive":
			item.Status = ItemStatusPassive
		case "NeedsAttention":
			item.Status = ItemStatusNeedsAttention
		default:
			item.Status = ItemStatusActive
		}
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

// uniqueNameAndPathFromDBusSignal retrieves unique name of the StatusNotifierItem
// service from D-Bus signal.
func uniqueNameAndPathFromDBusSignal(signal *dbus.Signal) (string, string, error) {
	if len(signal.Body) < 1 {
		return "", "", fmt.Errorf("signal body is empty")
	}

	itemName, ok := signal.Body[0].(string)
	if !ok {
		return "", "", fmt.Errorf("invalid format of signal body")
	}

	return uniqueNameAndPathFromItemName(itemName)
}

// uniqueNameAndPathFromItemName returns unique name and object path of the
// StatusNotifierItem service from its item name. The returned object path
// contains /.
//
// Format of item name is "<uniqueName>/<objectPath>",
// e.g. ":1.185/StatusNotifierItem".
func uniqueNameAndPathFromItemName(itemName string) (string, string, error) {
	uniqueName, objectPath, ok := strings.Cut(itemName, "/")
	if !ok {
		return uniqueName, StatusNotifierItemPath, nil
	}

	return uniqueName, "/" + objectPath, nil
}
