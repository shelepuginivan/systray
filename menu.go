package systray

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const MenuInterface = "com.canonical.dbusmenu"

// UpdatedProperties represents updated properties of a specific layout node.
type UpdatedProperties struct {
	// ID of the layout node.
	NodeID int32

	// Updated properties.
	Properties map[string]any
}

// getUpdatedProperties retrieves updated properties from the first argument of
// the com.canonical.dbusmenu.ItemsPropertiesUpdated signal.
func getUpdatedProperties(data any) ([]*UpdatedProperties, error) {
	items, ok := data.([][]any)
	if !ok {
		return nil, fmt.Errorf("invalid argument format")
	}

	updatedProperties := make([]*UpdatedProperties, 0, len(items))

	for _, item := range items {
		if len(item) != 2 {
			continue
		}

		nodeID, ok := item[0].(int32)
		if !ok {
			continue
		}

		props, ok := item[1].(map[string]dbus.Variant)
		if !ok {
			continue
		}

		up := &UpdatedProperties{
			NodeID:     nodeID,
			Properties: make(map[string]any, len(props)),
		}

		for key, prop := range props {
			up.Properties[key] = prop.Value()
		}

		updatedProperties = append(updatedProperties, up)
	}

	return updatedProperties, nil
}

// RemovedProperties represents removed properties of a specific layout node.
type RemovedProperties struct {
	// ID of the layout node.
	NodeID int32

	// Removed properties.
	Properties []string
}

// getRemovedProperties retrieves removed properties from the second argument
// of the com.canonical.dbusmenu.ItemsPropertiesUpdated signal.
func getRemovedProperties(data any) ([]*RemovedProperties, error) {
	items, ok := data.([][]any)
	if !ok {
		return nil, fmt.Errorf("invalid argument format")
	}

	removedProperties := make([]*RemovedProperties, 0, len(items))

	for _, item := range items {
		if len(item) != 2 {
			continue
		}

		nodeID, ok := item[0].(int32)
		if !ok {
			continue
		}

		props, ok := item[1].([]string)
		if !ok {
			continue
		}

		removedProperties = append(removedProperties, &RemovedProperties{
			NodeID:     nodeID,
			Properties: props,
		})
	}

	return removedProperties, nil
}

// Menu is a menu associated with [Item]. It implements the
// com.canonical.dbusmenu interface.
type Menu struct {
	uniqueName         string
	conn               *dbus.Conn
	signals            chan *dbus.Signal
	object             dbus.BusObject
	onLayoutUpdate     func(int32)
	onPropertiesUpdate func([]*UpdatedProperties, []*RemovedProperties)
	onActivate         func(int32)

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
		uniqueName:     name,
		conn:           conn,
		signals:        make(chan *dbus.Signal),
		object:         obj,
		onLayoutUpdate: func(int32) {},
		onActivate:     func(int32) {},
	}

	version, err := obj.GetProperty(MenuInterface + ".Version")
	if err == nil {
		version.Store(&menu.Version)
	}

	status, err := obj.GetProperty(MenuInterface + ".Status")
	if err == nil {
		status.Store(&menu.Status)
	}

	if err := menu.subscribe(); err != nil {
		return nil, fmt.Errorf("menu: %w", err)
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

// OnLayoutUpdate registers callback that runs whenever menu layout is updated.
//
// Parameter id of the callback is ID of the parent node for the nodes that
// have changed. If it is zero, the entire layout is updated.
func (m *Menu) OnLayoutUpdate(callback func(id int32)) {
	m.onLayoutUpdate = callback
}

// OnPropertiesUpdate registers callback that runs whenever properties of
// layout nodes are updated.
func (m *Menu) OnPropertiesUpdate(callback func(updated []*UpdatedProperties, removed []*RemovedProperties)) {
	m.onPropertiesUpdate = callback
}

// OnActivate registers a callback that runs whenever application requests to
// open the menu.
//
// Parameter id of callback is ID of a specific node that should be activated.
func (m *Menu) OnActivate(callback func(id int32)) {
	m.onActivate = callback
}

// Close unsubscribes from menu update signals.
func (m *Menu) Close() error {
	if err := m.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(MenuInterface),
		dbus.WithMatchMember("ItemsPropertiesUpdated"),
		dbus.WithMatchSender(m.uniqueName),
	); err != nil {
		return err
	}

	if err := m.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(MenuInterface),
		dbus.WithMatchMember("LayoutUpdated"),
		dbus.WithMatchSender(m.uniqueName),
	); err != nil {
		return err
	}

	if err := m.conn.RemoveMatchSignal(
		dbus.WithMatchInterface(MenuInterface),
		dbus.WithMatchMember("ItemActivationRequested"),
		dbus.WithMatchSender(m.uniqueName),
	); err != nil {
		return err
	}

	m.conn.RemoveSignal(m.signals)
	close(m.signals)

	m.onLayoutUpdate = nil
	m.onPropertiesUpdate = nil
	m.onActivate = nil

	return nil
}

// subscribe subscribes to signals
//   - com.canonical.dbusmenu.ItemsPropertiesUpdated
//   - com.canonical.dbusmenu.LayoutUpdated
//   - com.canonical.dbusmenu.ItemActivationRequested
func (m *Menu) subscribe() error {
	if err := m.conn.AddMatchSignal(
		dbus.WithMatchInterface(MenuInterface),
		dbus.WithMatchMember("ItemsPropertiesUpdated"),
		dbus.WithMatchSender(m.uniqueName),
	); err != nil {
		return err
	}

	if err := m.conn.AddMatchSignal(
		dbus.WithMatchInterface(MenuInterface),
		dbus.WithMatchMember("LayoutUpdated"),
		dbus.WithMatchSender(m.uniqueName),
	); err != nil {
		return err
	}

	if err := m.conn.AddMatchSignal(
		dbus.WithMatchInterface(MenuInterface),
		dbus.WithMatchMember("ItemActivationRequested"),
		dbus.WithMatchSender(m.uniqueName),
	); err != nil {
		return err
	}

	m.conn.Signal(m.signals)

	go func() {
		for signal := range m.signals {
			if signal.Sender != m.uniqueName {
				continue
			}

			switch signal.Name {
			case MenuInterface + ".ItemsPropertiesUpdated":
				m.handleItemPropertiesUpdated(signal)
			case MenuInterface + ".LayoutUpdated":
				m.handleLayoutUpdated(signal)
			case MenuInterface + ".ItemActivationRequested":
				m.handleItemActivationRequested(signal)
			}
		}
	}()

	return nil
}

// handleItemPropertiesUpdated handles the
// com.canonical.dbusmenu.ItemsPropertiesUpdated signal.
func (m *Menu) handleItemPropertiesUpdated(signal *dbus.Signal) {
	if len(signal.Body) != 2 {
		return
	}

	updatedProperties, err := getUpdatedProperties(signal.Body[0])
	if err != nil {
		return
	}

	removedProperties, err := getRemovedProperties(signal.Body[1])
	if err != nil {
		return
	}

	m.onPropertiesUpdate(updatedProperties, removedProperties)
}

// handleLayoutUpdated handles the
// com.canonical.dbusmenu.LayoutUpdated signal.
func (m *Menu) handleLayoutUpdated(signal *dbus.Signal) {
	if len(signal.Body) != 2 {
		return
	}

	nodeID, ok := signal.Body[1].(int32)
	if !ok {
		return
	}

	m.onLayoutUpdate(nodeID)
}

// handleItemActivationRequested handles the
// com.canonical.dbusmenu.ItemActivationRequested signal.
func (m *Menu) handleItemActivationRequested(signal *dbus.Signal) {
	if len(signal.Body) != 2 {
		return
	}

	nodeID, ok := signal.Body[0].(int32)
	if !ok {
		return
	}

	m.onActivate(nodeID)
}
