// os_service.go — the Go half of cljg.os service management (ADR 0088 §3): run
// a binary as an OS service across systemd (Linux), launchd (macOS), and the
// Windows SCM. The RENDER of the unit/plist text is pure and unit-tested; the
// install/start/stop/status/uninstall operations shell out to the platform's
// service tool (systemctl / launchctl / sc.exe) — thin, s48/s49-proven
// cgo-free, and not exercised in CI (no init system on the runners). Pure Go
// (os/exec + text) so CGO_ENABLED=0 + cljgo dist hold. Interned into cljg.os.
package bri

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// installServiceShims adds the service primitives to cljg.os (alongside the
// cron shims from installOSShims).
func installServiceShims(def func(name string, fn func(args ...any) any)) {
	// -service-render (spec goos) -> the unit/plist text for goos ("" = host).
	def("-service-render", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-service-render expects 2 args (spec os), got %d", len(args)))
		}
		goos := asString(args[1])
		if goos == "" {
			goos = runtime.GOOS
		}
		return renderService(readServiceSpec(args[0]), goos)
	})
	// -service-install (spec) -> install on the host platform.
	def("-service-install", func(args ...any) any {
		installService(readServiceSpec(one("-service-install", args)))
		return nil
	})
	// -service-op (op name) -> start|stop|status|uninstall on the host.
	def("-service-op", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-service-op expects 2 args (op name), got %d", len(args)))
		}
		return serviceOp(asString(args[0]), asString(args[1]))
	})
}

// serviceSpec is the platform-neutral description read from the cljgo map.
type serviceSpec struct {
	name, description, exec, workingDir, scope string
	args                                       []string
	env                                        [][2]string
}

func readServiceSpec(v any) serviceSpec {
	m, ok := v.(lang.IPersistentMap)
	if !ok {
		panic(fmt.Errorf("cljg.os: a service spec must be a map, got: %s", lang.PrintString(v)))
	}
	get := func(k string) any { return lang.Get(m, lang.NewKeyword(k)) }
	str := func(k string) string {
		if s, ok := get(k).(string); ok {
			return s
		}
		return ""
	}
	s := serviceSpec{
		name:        str("name"),
		description: str("description"),
		exec:        str("exec"),
		workingDir:  str("working-dir"),
		scope:       "user",
	}
	if sc, ok := get("scope").(lang.Keyword); ok {
		s.scope = keywordName(sc)
	}
	if s.name == "" || s.exec == "" {
		panic(fmt.Errorf("cljg.os: a service spec needs :name and :exec"))
	}
	for seq := lang.Seq(get("args")); seq != nil; seq = lang.Next(seq) {
		s.args = append(s.args, lang.ToString(lang.First(seq)))
	}
	if em, ok := get("env").(lang.IPersistentMap); ok {
		for seq := lang.Seq(em); seq != nil; seq = lang.Next(seq) {
			e := lang.First(seq)
			k := lang.First(e)
			var key string
			if kk, ok := k.(lang.Keyword); ok {
				key = keywordName(kk)
			} else {
				key = lang.ToString(k)
			}
			s.env = append(s.env, [2]string{key, lang.ToString(lang.Get(e, int64(1)))})
		}
	}
	return s
}

func (s serviceSpec) execLine() string {
	parts := append([]string{s.exec}, s.args...)
	return strings.Join(parts, " ")
}

// renderService produces the exact file (systemd unit / launchd plist) install
// would write. Windows has no such file (the SCM is API/sc.exe-based).
func renderService(s serviceSpec, goos string) string {
	switch goos {
	case "linux":
		return renderSystemd(s)
	case "darwin":
		return renderLaunchd(s)
	case "windows":
		return "" // SCM is not file-based; install uses sc.exe
	default:
		panic(fmt.Errorf("cljg.os: no service backend for GOOS %q", goos))
	}
}

func renderSystemd(s serviceSpec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=%s\n\n[Service]\n", firstNonEmptyStr(s.description, s.name))
	fmt.Fprintf(&b, "ExecStart=%s\n", s.execLine())
	if s.workingDir != "" {
		fmt.Fprintf(&b, "WorkingDirectory=%s\n", s.workingDir)
	}
	for _, kv := range s.env {
		fmt.Fprintf(&b, "Environment=%s=%s\n", kv[0], kv[1])
	}
	b.WriteString("Restart=on-failure\n\n[Install]\n")
	target := "default.target"
	if s.scope == "system" {
		target = "multi-user.target"
	}
	fmt.Fprintf(&b, "WantedBy=%s\n", target)
	return b.String()
}

func renderLaunchd(s serviceSpec) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "  <key>Label</key>\n  <string>%s</string>\n", xmlEscape(s.name))
	b.WriteString("  <key>ProgramArguments</key>\n  <array>\n")
	for _, a := range append([]string{s.exec}, s.args...) {
		fmt.Fprintf(&b, "    <string>%s</string>\n", xmlEscape(a))
	}
	b.WriteString("  </array>\n")
	if len(s.env) > 0 {
		b.WriteString("  <key>EnvironmentVariables</key>\n  <dict>\n")
		for _, kv := range s.env {
			fmt.Fprintf(&b, "    <key>%s</key>\n    <string>%s</string>\n", xmlEscape(kv[0]), xmlEscape(kv[1]))
		}
		b.WriteString("  </dict>\n")
	}
	if s.workingDir != "" {
		fmt.Fprintf(&b, "  <key>WorkingDirectory</key>\n  <string>%s</string>\n", xmlEscape(s.workingDir))
	}
	b.WriteString("  <key>RunAtLoad</key>\n  <true/>\n  <key>KeepAlive</key>\n  <true/>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// --- the operations (shell-outs to the platform service tool) ---------------

func svcRun(name string, argv ...string) (string, error) {
	out, err := exec.Command(name, argv...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("cljg.os: %s %s: %w: %s", name, strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func systemdUserFlag(s serviceSpec) []string {
	if s.scope == "system" {
		return nil
	}
	return []string{"--user"}
}

func installService(s serviceSpec) {
	text := renderService(s, runtime.GOOS)
	switch runtime.GOOS {
	case "linux":
		path := systemdUnitPath(s)
		writeServiceFile(path, text)
		flags := systemdUserFlag(s)
		svcMustRun("systemctl", append(flags, "daemon-reload")...)
		svcMustRun("systemctl", append(flags, "enable", "--now", s.name)...)
	case "darwin":
		path := launchdPlistPath(s)
		writeServiceFile(path, text)
		svcMustRun("launchctl", "load", "-w", path)
	case "windows":
		svcMustRun("sc.exe", "create", s.name, "binPath=", s.execLine(), "start=", "auto")
		svcMustRun("sc.exe", "start", s.name)
	default:
		panic(fmt.Errorf("cljg.os: no service backend for %q", runtime.GOOS))
	}
}

func serviceOp(op, name string) any {
	switch runtime.GOOS {
	case "linux":
		switch op {
		case "start":
			svcMustRun("systemctl", "--user", "start", name)
		case "stop":
			svcMustRun("systemctl", "--user", "stop", name)
		case "uninstall":
			_, _ = svcRun("systemctl", "--user", "disable", "--now", name)
			_ = os.Remove(filepath.Join(userSystemdDir(), name+".service"))
		case "status":
			out, err := svcRun("systemctl", "--user", "is-active", name)
			if err != nil {
				return nil
			}
			return out
		}
	case "darwin":
		switch op {
		case "start":
			svcMustRun("launchctl", "start", name)
		case "stop":
			svcMustRun("launchctl", "stop", name)
		case "uninstall":
			_, _ = svcRun("launchctl", "unload", "-w", launchdPlistPathByName(name))
			_ = os.Remove(launchdPlistPathByName(name))
		case "status":
			out, err := svcRun("launchctl", "list", name)
			if err != nil {
				return nil
			}
			return out
		}
	case "windows":
		switch op {
		case "start":
			svcMustRun("sc.exe", "start", name)
		case "stop":
			svcMustRun("sc.exe", "stop", name)
		case "uninstall":
			_, _ = svcRun("sc.exe", "stop", name)
			svcMustRun("sc.exe", "delete", name)
		case "status":
			out, err := svcRun("sc.exe", "query", name)
			if err != nil {
				return nil
			}
			return out
		}
	default:
		panic(fmt.Errorf("cljg.os: no service backend for %q", runtime.GOOS))
	}
	return nil
}

func svcMustRun(name string, argv ...string) {
	if _, err := svcRun(name, argv...); err != nil {
		panic(err)
	}
}

func writeServiceFile(path, text string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(fmt.Errorf("cljg.os: %w", err))
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		panic(fmt.Errorf("cljg.os: %w", err))
	}
}

func userSystemdDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "systemd", "user")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func systemdUnitPath(s serviceSpec) string {
	if s.scope == "system" {
		return filepath.Join("/etc/systemd/system", s.name+".service")
	}
	return filepath.Join(userSystemdDir(), s.name+".service")
}

func launchdPlistPathByName(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", name+".plist")
}

func launchdPlistPath(s serviceSpec) string { return launchdPlistPathByName(s.name) }
