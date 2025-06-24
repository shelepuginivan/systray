package systray

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const MenuInterface = "com.canonical.dbusmenu"

// Menu is a menu associated with [Item]. It implements the
// com.canonical.dbusmenu interface.
type Menu struct {
	object dbus.BusObject

	// Version of the com.canonical.dbusmenu interface.
	Version uint

	// Status of the application, whether it requires attention. Possible values
	// are "normal" (for most cases) and "notice" (a higher priority to be shown).
	Status string
}

// NewMenu retrieves menu of item with specified name and path.
func NewMenu(conn *dbus.Conn, name, path string) (*Menu, error) {
	obj := conn.Object(name, dbus.ObjectPath(path))

	// Check whether properties can be retrieved.
	call := obj.Call(getProperty, dbus.Flags(64), MenuInterface, "Version")
	if call.Err != nil {
		return nil, fmt.Errorf("failed to retrieve menu: %w", call.Err)
	}

	menu := Menu{
		object: obj,
	}

	version, err := obj.GetProperty(MenuInterface + ".Version")
	if err == nil {
		version.Store(&menu.Version)
	}

	status, err := obj.GetProperty(MenuInterface + ".Status")
	if err == nil {
		status.Store(&menu.Status)
	}

	return &menu, nil
}

// GetLayout provides the layout and propertiers that are attached to the
// entries that are in the layout.
//
// parentID is the ID of the parent node for the returned layout. Use 0 to
// retrieve layout from root.
//
// recursionDepth is the number of recursion levels to use. This affects the
// resulting [LayoutNode]. Special cases are:
//   - -1: deliver all items (without recursion limit).
//   - 0: disable recursion (children slice will be empty).
//
// propertyNames is the list of properties associated with layout nodes.
// Special case is empty slice (or nil): all properties are returned.
func (m *Menu) GetLayout(parentID int, recursionDepth int, propertyNames []string) (uint32, *LayoutNode, error) {
	call := m.object.Call(
		MenuInterface+".GetLayout",
		dbus.Flags(64),
		parentID, recursionDepth, propertyNames,
	)

	if call.Err != nil {
		return 0, nil, call.Err
	}

	if len(call.Body) != 2 {
		return 0, nil, fmt.Errorf("layout: invalid response body format")
	}

	revision, ok := call.Body[0].(uint32)
	if !ok {
		return 0, nil, fmt.Errorf("layout: invalid revision type")
	}

	menu, err := NewLayoutNode(call.Body[1])
	if err != nil {
		return revision, nil, fmt.Errorf("layout: %w", err)
	}

	return revision, menu, nil
}

// Clicked tells the application that the target layout node was clicked.
func (m *Menu) Clicked(target *LayoutNode) error {
	return m.Event(target.ID, "clicked", 0, uint32(time.Now().Unix()))
}

// Hovered tells the application that the target layout node was hovered.
func (m *Menu) Hovered(target *LayoutNode) error {
	return m.Event(target.ID, "hovered", 0, uint32(time.Now().Unix()))
}

// Event tells the application that an arbitrary event happened to layout node
// with the given ID.
//
// Possible values for eventID are:
//   - clicked
//   - hovered
//
// Vendor-specific events can be sent by prefixing eventID with "x-<vendor>-".
func (m *Menu) Event(targetID int32, eventID string, data any, timestamp uint32) error {
	return m.object.Call(
		MenuInterface+".Event",
		dbus.Flags(64),
		targetID,
		eventID,
		dbus.MakeVariant(data),
		timestamp,
	).Err
}

// AboutToShow tells the application that target layout node is about to be
// shown by the applet.
func (m *Menu) AboutToShow(target *LayoutNode) (bool, error) {
	call := m.object.Call(
		MenuInterface+".AboutToShow",
		dbus.Flags(64),
		target.ID,
	)

	if call.Err != nil {
		return false, fmt.Errorf("about to show: %w", call.Err)
	}

	if len(call.Body) != 1 {
		return false, fmt.Errorf("about to show: invalid response format")
	}

	needUpdate, ok := call.Body[0].(bool)
	if !ok {
		return false, fmt.Errorf("about to show: invalid response format")
	}

	return needUpdate, nil
}
