package web

import _ "embed"

// DashboardHTML is the collector UI.
//
//go:embed static/index.html
var DashboardHTML []byte
