package functools

import (
	"context"
	"encoding/json"
	"net"
	"os"

	"WorkBaby/internal/tool"
)

// ipLookupTool 返回本机主机名与 IP 地址。
func ipLookupTool() tool.Tool {
	return New(
		"ip_lookup",
		"Returns the hostname and IP address of the local machine.",
		tool.RiskReadOnly,
		`{"type": "object", "properties": {}}`,
		func(_ context.Context, _ json.RawMessage) (any, error) {
			host := "unknown"
			if h, err := os.Hostname(); err == nil {
				host = h
			}
			ips := []string{}
			if addrs, err := net.InterfaceAddrs(); err == nil {
				for _, a := range addrs {
					if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
						ips = append(ips, ipnet.IP.String())
					}
				}
			}
			return map[string]any{"hostname": host, "ips": ips}, nil
		},
	)
}
