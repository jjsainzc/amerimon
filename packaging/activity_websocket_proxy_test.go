package packaging

import (
	"os"
	"strings"
	"testing"
)

func TestActivityWebSocketProxyConfigurations(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		upstream      string
		fallback      string
		requiredLines []string
	}{
		{
			name:     "Apache",
			path:     "apache-amerimon.conf.example",
			upstream: "ws://127.0.0.1:8082/mon/allstarlink/activity/live",
			fallback: "ProxyPass        /amerimon/ http://127.0.0.1:8082/",
			requiredLines: []string{
				"ProxyPass        /amerimon/mon/allstarlink/activity/live",
				"ProxyPassReverse /amerimon/mon/allstarlink/activity/live",
			},
		},
		{
			name:     "Nginx",
			path:     "nginx-amerimon.conf.example",
			upstream: "http://127.0.0.1:8082/mon/allstarlink/activity/live",
			fallback: "location /amerimon/ {",
			requiredLines: []string{
				"proxy_http_version 1.1;",
				"proxy_set_header Upgrade $http_upgrade;",
				`proxy_set_header Connection "upgrade";`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			config := string(data)
			route := strings.Index(config, "/amerimon/mon/allstarlink/activity/live")
			fallback := strings.Index(config, tc.fallback)
			if route < 0 || fallback < 0 || route >= fallback {
				t.Fatalf("activity WebSocket route must precede the HTTP fallback in %s", tc.path)
			}
			if !strings.Contains(config, tc.upstream) {
				t.Errorf("activity WebSocket upstream %q missing from %s", tc.upstream, tc.path)
			}
			for _, line := range tc.requiredLines {
				if !strings.Contains(config, line) {
					t.Errorf("required proxy directive %q missing from %s", line, tc.path)
				}
			}
		})
	}

	for _, tc := range []struct {
		path  string
		lines []string
	}{
		{
			path: "postinstall.sh",
			lines: []string{
				"APACHE_NEED_ACTIVITY_PROXY=1",
				"APACHE_NEED_ACTIVITY_PROXY_REVERSE=1",
				"ProxyPass        /amerimon/mon/allstarlink/activity/live ws://127.0.0.1:8082/mon/allstarlink/activity/live",
				"ProxyPassReverse /amerimon/mon/allstarlink/activity/live ws://127.0.0.1:8082/mon/allstarlink/activity/live",
			},
		},
		{
			path: "configure-nginx.sh",
			lines: []string{
				"ACTIVITY_WEBSOCKET_MISSING=0",
				"location ^~ /amerimon/mon/allstarlink/activity/live",
				"proxy_set_header Upgrade $http_upgrade;",
			},
		},
	} {
		data, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range tc.lines {
			if !strings.Contains(string(data), line) {
				t.Errorf("activity WebSocket proxy migration directive %q missing from %s", line, tc.path)
			}
		}
	}
}
