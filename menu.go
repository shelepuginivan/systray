package systray

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const MenuInterface = "com.canonical.dbusmenu"

type Menu struct {
	object dbus.BusObject

	Version uint
	Status  string
}

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

func (m *Menu) Clicked(target *LayoutNode) error {
	return m.Event(target.ID, "clicked", 0, uint32(time.Now().Unix()))
}

func (m *Menu) Hovered(target *LayoutNode) error {
	return m.Event(target.ID, "hovered", 0, uint32(time.Now().Unix()))
}

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
