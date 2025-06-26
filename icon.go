package systray

import (
	"fmt"
	"sort"
)

// Icon represents icon of the system tray item.
type Icon struct {
	// Width of the icon.
	Width int32

	// Height of the icon.
	Height int32

	// ARGB32 binary representation of the icon.
	Bytes []byte
}

// IconSet represents a set of resolutions for an icon.
type IconSet struct {
	icons []*Icon
}

// NewIconFromDBusPixmap returns a new [Icon] from D-Bus pixmap.
//
// Format of pixmap is as follows
//
//	[<width>, <height>, <bytes>]
//
// Where:
//   - <width>: width of the icon (int32)
//   - <height>: height of the icon (int32)
//   - <bytes>: content of the icon ([]byte)
func NewIconFromDBusPixmap(pixmap any) (*Icon, error) {
	data, ok := pixmap.([]any)
	if !ok || len(data) != 3 {
		return nil, fmt.Errorf("invalid pixmap format: expected a slice of 3 elements")
	}

	width, ok := data[0].(int32)
	if !ok {
		return nil, fmt.Errorf("invalid width type: expected int32")
	}

	height, ok := data[1].(int32)
	if !ok {
		return nil, fmt.Errorf("invalid height type: expected int32")
	}

	bytes, ok := data[2].([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid bytes format: expected []byte")
	}

	return &Icon{
		Width:  width,
		Height: height,
		Bytes:  bytes,
	}, nil
}

// NewIconSetFromDBusProperty returns a new [IconSet] from value of D-Bus icon
// properties, such as
//   - IconPixmap
//   - OverlayIconPixmap
//   - AttentionIconPixmap
//
// Format of value is as follows
//
//	[<icon>]
//
// See [NewIconFromDBusPixmap] for details about <icon> format.
func NewIconSetFromDBusProperty(value any) (*IconSet, error) {
	pixmaps, ok := value.([][]any)
	if !ok {
		return nil, fmt.Errorf("invalid property format: expected a slice of slices")
	}

	icons := make([]*Icon, 0, len(pixmaps))

	for _, pixmap := range pixmaps {
		icon, err := NewIconFromDBusPixmap(pixmap)
		if err != nil {
			fmt.Println(err)
			continue
		}

		icons = append(icons, icon)
	}

	sort.Slice(icons, func(i, j int) bool {
		a := icons[i]
		b := icons[j]

		return a.Width*a.Height < b.Width*b.Height
	})

	return &IconSet{
		icons: icons,
	}, nil
}

// GetAll returns all resolutions in the set.
func (is *IconSet) GetAll() []*Icon {
	return is.icons
}

// GetSmallest returns the smallest icon in the set.
func (is *IconSet) GetSmallest() *Icon {
	if len(is.icons) == 0 {
		return nil
	}

	return is.icons[0]
}

// GetLargest returns the largest icon in the set.
func (is *IconSet) GetLargest() *Icon {
	if len(is.icons) == 0 {
		return nil
	}

	return is.icons[len(is.icons)-1]
}
