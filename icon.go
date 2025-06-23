package systray

import "fmt"

// Icon represents icon of the system tray item.
type Icon struct {
	Width  int32
	Height int32
	Bytes  []byte
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
