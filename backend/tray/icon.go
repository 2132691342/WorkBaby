package tray

import _ "embed"

// 托盘与应用窗口共用同一枚应用图标；Windows 托盘缺省没有图标，必须显式注入。
//
//go:embed icon.ico
var iconICO []byte
