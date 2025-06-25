package systray

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

type LayoutNodeToggleType string

// [LayoutNode] toggle types.
const (
	LayoutNodeToggleTypeNone      LayoutNodeToggleType = ""
	LayoutNodeToggleTypeCheckmark LayoutNodeToggleType = "checkmark"
	LayoutNodeToggleTypeRadio     LayoutNodeToggleType = "radio"
)

type LayoutNodeToggleState int

// [LayoutNode] toggle states.
const (
	LayoutNodeToggleStateIndeterminate LayoutNodeToggleState = iota - 1
	LayoutNodeToggleStateOff
	LayoutNodeToggleStateOn
)

func (ts LayoutNodeToggleState) String() string {
	switch ts {
	case LayoutNodeToggleStateOff:
		return "off"
	case LayoutNodeToggleStateOn:
		return "on"
	default:
		return "indeterminate"
	}
}

// LayoutNode represents entry of the menu layout.
//
// The menu layout is recursive.
type LayoutNode struct {
	ID         int32
	Properties map[string]any
	Children   []*LayoutNode
}

// NewLayoutNode parses menu layout from data.
func NewLayoutNode(data any) (*LayoutNode, error) {
	arr, ok := data.([]any)
	if !ok || len(arr) != 3 {
		return nil, fmt.Errorf("menu node: invalid format")
	}

	id, ok := arr[0].(int32)
	if !ok {
		return nil, fmt.Errorf("menu node: invalid id")
	}

	props, ok := arr[1].(map[string]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("menu node: invalid props")
	}

	children, ok := arr[2].([]dbus.Variant)
	if !ok {
		return nil, fmt.Errorf("menu node: invalid children")
	}

	root := &LayoutNode{
		ID:         id,
		Properties: make(map[string]any, len(props)),
		Children:   make([]*LayoutNode, 0, len(children)),
	}

	for key, value := range props {
		root.Properties[key] = value.Value()
	}

	for _, child := range children {
		childNode, err := NewLayoutNode(child.Value())
		if err != nil {
			continue
		}

		root.Children = append(root.Children, childNode)
	}

	return root, nil
}

// IsEnabled reports whether layout node can be the target of events, e.g. can
// be clicked or hovered.
func (node *LayoutNode) IsEnabled() bool {
	enabled, exists := node.Properties["enabled"]
	if !exists {
		return true // Enabled by default.
	}
	return enabled == true
}

// IsSeparator reports whether layout node is a separator.
func (node *LayoutNode) IsSeparator() bool {
	nodeType, exists := node.Properties["type"]
	return exists && nodeType == "separator"
}

// IsSubmenu reports whether layout node is a submenu.
func (node *LayoutNode) IsSubmenu() bool {
	childrenDisplay, exists := node.Properties["children-display"]
	return exists && childrenDisplay == "submenu"
}

// IsVisible reports whether layout node is visible in the menu.
func (node *LayoutNode) IsVisible() bool {
	visible, exists := node.Properties["visible"]
	if !exists {
		return true // Visible by default.
	}
	return visible == true
}

// Label returns text label of layout node if exists.
//
//   - Two consecutive underscore characters "__" should be displayed as a
//     single underscore.
//   - Any remaining underscore characters should not displayed at all, the
//     first of those remaining underscore characters (unless it is the last
//     character in the string) indicates that the following character is the
//     access key.
func (node *LayoutNode) Label() string {
	nodeLabel, exists := node.Properties["label"]
	if !exists {
		return ""
	}

	label, ok := nodeLabel.(string)
	if !ok {
		return ""
	}

	return label
}

// IconName returns name of the icon, following the
// [Freedesktop Icon Naming Specification].
//
// [Freedesktop Icon Naming Specification]: https://specifications.freedesktop.org/icon-naming-spec/latest/
func (node *LayoutNode) IconName() string {
	nodeIconName, exists := node.Properties["icon-name"]
	if !exists {
		return ""
	}

	iconName, ok := nodeIconName.(string)
	if !ok {
		return ""
	}

	return iconName
}

// IconData returns PNG bytes of the custom icon.
func (node *LayoutNode) IconData() []byte {
	nodeIconData, exists := node.Properties["icon-data"]
	if !exists {
		return nil
	}

	iconData, ok := nodeIconData.([]byte)
	if !ok {
		return nil
	}

	return iconData
}

// ToggleType returns toggle type of the layout node.
func (node *LayoutNode) ToggleType() LayoutNodeToggleType {
	switch node.Properties["toggle-type"] {
	case "checkmark":
		return LayoutNodeToggleTypeCheckmark
	case "radio":
		return LayoutNodeToggleTypeRadio
	default:
		return LayoutNodeToggleTypeNone
	}
}

// ToggleState describes the current state of a togglable item.
//
// The implementation does not itself handle ensuring that only one item in a
// radio group is set to "on", or that a group does not have "on" and
// "indeterminate" items simultaneously; maintaining this policy is up to the
// toolkit wrappers.
func (node *LayoutNode) ToggleState() LayoutNodeToggleState {
	switch node.Properties["toggle-state"] {
	case 0, int32(0):
		return LayoutNodeToggleStateOff
	case 1, int32(1):
		return LayoutNodeToggleStateOn
	default:
		return LayoutNodeToggleStateIndeterminate
	}
}
