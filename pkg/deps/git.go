package deps

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// git runs a git subcommand in dir with prompts disabled and system config
// ignored (hermetic, no credential helpers, no interactive auth).
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// resolveRef turns a ref (tag/branch/HEAD) into an immutable SHA by asking the
// remote. This is the ONLY step that needs the network; a locked build skips it
// entirely (ADR 0052 §1). A 40-hex ref is already an identity.
func resolveRef(url, ref string) (string, error) {
	out, err := git("", "ls-remote", url, ref)
	if err != nil {
		if len(ref) == 40 && isHex(ref) {
			return ref, nil
		}
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			return f[0], nil
		}
	}
	if len(ref) == 40 && isHex(ref) {
		return ref, nil
	}
	return "", fmt.Errorf("ref %q not found at %s", ref, url)
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
