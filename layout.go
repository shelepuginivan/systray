package systray

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

type LayoutNode struct {
	ID         int32
	Properties map[string]any
	Children   []*LayoutNode
}

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
