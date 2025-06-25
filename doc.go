// Package systray is a toolkit-agnostic implementation of the
// [StatusNotifierItem] specification. It provides services for system tray
// hosts. This package does not provide capabilities for system tray
// applications (clients), it is intended to be used for building system trays
// themselves.
//
// # Usage
//
// System tray consists of [Watcher], [Host], and multiple [Item] instances:
//   - [Watcher] keeps track of tray items and hosts. One watcher must be
//     present on a D-Bus at a time.
//   - [Host] stores tray items and provides access to them. It requires a
//     watcher service instance to be registered on the session bus (either
//     [Watcher] or an external implementation can be used).
//   - [Item] is the application running in the system tray.
//
// In addition to the base specification, package systray implements
// com.canonical.dbusmenu, providing support for tray item menus.
//
// [StatusNotifierItem]: https://www.freedesktop.org/wiki/Specifications/StatusNotifierItem/
package systray
