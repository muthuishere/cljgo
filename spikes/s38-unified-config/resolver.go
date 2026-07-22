package main

import (
	"strings"
)

// Layer names, low precedence -> high. A key present at a higher layer wins.
const (
	LayerDefault    = "default" // built-in defaults
	LayerFile       = "file"    // application.{edn,properties}
	LayerProfile    = "profile" // application-{profile}.{edn,properties}
	LayerEnv        = "env"     // APP_* environment
	LayerVault      = "vault"   // secrets backend (stub)
	LayerRuntime    = "runtime" // DB-primary override (wins over all)
	LayerPrecedence = "default<file<profile<env<vault<runtime"
)

// Sources bundles everything the resolver composes. Any field may be
// nil/empty; a missing layer simply contributes nothing.
type Sources struct {
	Defaults Map             // built-in defaults (layer 1)
	FileDir  string          // dir holding application.* + application-{profile}.*
	Profile  string          // APP_PROFILE value (selects the profile file)
	Env      []EnvPair       // APP_* pairs (layer 4)
	Vault    Vault           // layer 5 (stub interface)
	Runtime  *RuntimeStore   // layer 6, DB-primary
	Secrets  map[string]bool // extra secret registry (join-dot key -> true)
}

// EnvPair is a raw environment [name value] pair, matching bri's -env-pairs.
type EnvPair struct{ Name, Value string }

// Resolved is the outcome: the merged config, per-leaf provenance, and the set
// of secret leaf paths (for masking).
type Resolved struct {
	Config Map
	Source map[string]string // join-dot path -> winning layer
	Secret map[string]bool   // join-dot path -> is-secret
}

// Get is the config/get analogue.
func (r *Resolved) Get(path Path) (any, bool) { return getIn(r.Config, path) }

// SourceOf is the config/source analogue.
func (r *Resolved) SourceOf(path Path) string { return r.Source[joinDot(path)] }

// Resolve composes the six layers in precedence order and records, per leaf,
// which layer supplied the winning value.
func Resolve(s Sources) (Resolved, error) {
	// Track provenance as we merge: the last layer to set a leaf owns it.
	src := map[string]string{}
	secret := map[string]bool{}

	merged := Map{}

	apply := func(layer string, m Map) {
		if m == nil {
			return
		}
		merged = deepMerge(merged, m)
		for _, p := range leafPaths(m) {
			src[joinDot(p)] = layer
		}
	}

	// 1. defaults
	apply(LayerDefault, s.Defaults)

	// 2. application.{edn,properties}  (edn canonical; properties merged over
	//    edn if both exist, but a project uses one — see VERDICT).
	fileMap, err := readFilePair(s.FileDir, "application")
	if err != nil {
		return Resolved{}, err
	}
	apply(LayerFile, fileMap)

	// 3. application-{profile}.{edn,properties}
	if s.Profile != "" {
		profMap, err := readFilePair(s.FileDir, "application-"+s.Profile)
		if err != nil {
			return Resolved{}, err
		}
		apply(LayerProfile, profMap)
	}

	// 4. APP_* env
	apply(LayerEnv, envOverlay(s.Env))

	// 5. vault (stub) — only for paths it knows; also registers secret paths.
	if s.Vault != nil {
		for _, p := range s.Vault.SecretPaths() {
			secret[joinDot(p)] = true
			if v, ok := s.Vault.Lookup(p); ok {
				merged = setIn(merged, p, v)
				src[joinDot(p)] = LayerVault
			}
		}
	}

	// 6. DB-primary runtime override — wins over everything present.
	if s.Runtime != nil {
		for _, r := range s.Runtime.Rows() {
			merged = setIn(merged, r.Path, r.Row.Value)
			src[joinDot(r.Path)] = LayerRuntime
			if r.Row.IsSecret {
				secret[joinDot(r.Path)] = true
			}
		}
	}

	// explicit secret registry (e.g. keys known-secret regardless of layer)
	for k := range s.Secrets {
		secret[k] = true
	}

	return Resolved{Config: merged, Source: src, Secret: secret}, nil
}

// readFilePair loads base.edn then overlays base.properties (if any) into the
// canonical map. Both parse to the identical shape (criterion 2).
func readFilePair(dir, base string) (Map, error) {
	if dir == "" {
		return nil, nil
	}
	ednMap, err := readEDN(dir + "/" + base + ".edn")
	if err != nil {
		return nil, err
	}
	propMap, err := readProperties(dir + "/" + base + ".properties")
	if err != nil {
		return nil, err
	}
	if ednMap == nil {
		return propMap, nil
	}
	return deepMerge(ednMap, propMap), nil
}

// envOverlay turns APP_* pairs into a canonical map. Mapping matches the
// existing battery: __ separates path segments, _ joins words within a
// segment; values coerce to typed scalars. APP_PROFILE / APP_SESSION_KEY are
// selectors, not config data.
func envOverlay(pairs []EnvPair) Map {
	out := Map{}
	for _, p := range pairs {
		if !strings.HasPrefix(p.Name, "APP_") || p.Name == "APP_PROFILE" || p.Name == "APP_SESSION_KEY" {
			continue
		}
		out = setIn(out, envPath(p.Name[len("APP_"):]), coerceScalar(p.Value))
	}
	return out
}

// envPath: DB__POOL_SIZE -> ["db","pool-size"].
func envPath(rest string) Path {
	segs := strings.Split(rest, "__")
	p := make(Path, 0, len(segs))
	for _, seg := range segs {
		p = append(p, strings.ReplaceAll(strings.ToLower(seg), "_", "-"))
	}
	return p
}

// --- small path<->string helpers -------------------------------------------

func joinDot(p Path) string { return strings.Join(p, ".") }

func splitJoin(s string) Path {
	if s == "" {
		return nil
	}
	return strings.Split(s, ".")
}
