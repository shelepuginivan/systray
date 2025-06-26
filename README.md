# systray

[![Go Reference](https://pkg.go.dev/badge/github.com/shelepuginivan/systray.svg)](https://pkg.go.dev/github.com/shelepuginivan/systray)
[![Go Report Card](https://goreportcard.com/badge/github.com/shelepuginivan/systray)](https://goreportcard.com/report/github.com/shelepuginivan/systray)
[![License: MIT](https://img.shields.io/badge/License-MIT-00cc00.svg)](https://github.com/shelepuginivan/systray/blob/main/LICENSE)

Package `systray` is a toolkit-agnostic implementation of the
[StatusNotifierItem](https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/)
specification. It provides services for system tray hosts. This package does
not provide capabilities for system tray applications (clients), it is intended
to be used for building system trays themselves.

Package documentation is available at [pkg.go.dev](https://pkg.go.dev/github.com/shelepuginivan/systray).

## Installation

```shell
go get github.com/shelepuginivan/systray
```

## Example

```go
package main

import (
	"log"
	"os"

	"github.com/godbus/dbus/v5"
	"github.com/shelepuginivan/systray"
)

func main() {
	conn, _ := dbus.SessionBus()

	watcher := systray.NewWatcher(conn)
	host := systray.NewHost(conn, os.Getpid())

	watcher.RegisterHost(host)

	host.OnRegister(func(item *systray.Item) {
		log.Printf("%s (%s) is registered\n", item.Title, item.BusName())
	})

	host.OnUnregister(func(item *systray.Item) {
		log.Printf("%s (%s) is unregistered\n", item.Title, item.BusName())
	})

	if err := watcher.Listen(); err != nil {
		panic(err)
	}

	if err := host.Listen(); err != nil {
		panic(err)
	}

	select {}
}
```

## License

[MIT](https://github.com/shelepuginivan/systray/blob/main/LICENSE)
