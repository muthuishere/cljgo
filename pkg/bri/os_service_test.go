// os_service_test.go — white-box tests for cljg.os service RENDER (ADR 0088
// §3): the exact systemd unit / launchd plist text install would write. The
// install/start/stop operations shell out to the platform service tool and are
// not exercised here (no init system in CI); render is the pure, tested core.
package bri

import (
	"strings"
	"testing"
)

func TestRenderSystemd(t *testing.T) {
	s := serviceSpec{
		name: "mydaemon", description: "My Daemon", exec: "/usr/bin/mydaemon",
		args: []string{"--serve", "--port=8080"}, env: [][2]string{{"API_KEY", "x"}},
		workingDir: "/var/app", scope: "user",
	}
	out := renderService(s, "linux")
	for _, want := range []string{
		"[Unit]", "Description=My Daemon", "[Service]",
		"ExecStart=/usr/bin/mydaemon --serve --port=8080",
		"WorkingDirectory=/var/app", "Environment=API_KEY=x",
		"Restart=on-failure", "[Install]", "WantedBy=default.target",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("systemd unit missing %q:\n%s", want, out)
		}
	}
	// :system scope targets multi-user, not the user session
	s.scope = "system"
	if !strings.Contains(renderService(s, "linux"), "WantedBy=multi-user.target") {
		t.Errorf("system-scope unit should target multi-user.target")
	}
	// a missing :description falls back to :name
	s2 := serviceSpec{name: "svc", exec: "/bin/svc"}
	if !strings.Contains(renderService(s2, "linux"), "Description=svc") {
		t.Errorf("description should fall back to the name")
	}
}

func TestRenderLaunchd(t *testing.T) {
	s := serviceSpec{
		name: "com.x.daemon", exec: "/usr/local/bin/d", args: []string{"run"},
		env: [][2]string{{"K", "a&<b"}}, workingDir: "/tmp",
	}
	out := renderService(s, "darwin")
	for _, want := range []string{
		`<plist version="1.0">`, "<key>Label</key>", "<string>com.x.daemon</string>",
		"<key>ProgramArguments</key>", "<string>/usr/local/bin/d</string>", "<string>run</string>",
		"<key>K</key>", "<string>a&amp;&lt;b</string>", // XML-escaped env value
		"<key>WorkingDirectory</key>", "<string>/tmp</string>",
		"<key>RunAtLoad</key>", "<key>KeepAlive</key>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("launchd plist missing %q:\n%s", want, out)
		}
	}
}

func TestRenderWindowsIsSCMBased(t *testing.T) {
	if got := renderService(serviceSpec{name: "x", exec: "y"}, "windows"); got != "" {
		t.Errorf("windows render should be empty (SCM is not file-based), got %q", got)
	}
}
