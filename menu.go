package systray

import (
	"fmt"

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
