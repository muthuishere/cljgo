package main

import (
	"fmt"
	"reflect"
	"strings"
)

func main() {
	fmt.Println("s38 — unified layered config + DB-primary runtime override")
	fmt.Println("precedence:", LayerPrecedence)
	fmt.Println(strings.Repeat("=", 68))

	demo1And3Precedence()
	demo2SameMap()
	demo4HotReload()
	demo5Coercion()
}

// --- criteria 1 + 3: 6-layer precedence + provenance ------------------------

func buildSources() Sources {
	defaults := Map{
		"app":    Map{"name": "myapp"},
		"db":     Map{"pool-size": int64(5), "host": "localhost"},
		"log":    Map{"level": "warn"},
		"server": Map{"port": int64(3000)},
		"cache":  Map{"ttl-seconds": int64(300)},    // duration as NUMBER (crit 5)
		"upload": Map{"max-bytes": int64(10485760)}, // size as NUMBER (crit 5)
	}

	vault := newStubVault()
	vault.Put(Path{"db", "password"}, "s3cr3t-db-pw") // secret, only in vault

	rt := NewRuntimeStore()
	rt.Set(Path{"db", "pool-size"}, int64(50), false, "system:seed")         // tops out db.pool-size
	rt.Set(Path{"openai", "api-key"}, "sk-live-ABCDEF", true, "admin:muthu") // secret via runtime

	return Sources{
		Defaults: defaults,
		FileDir:  "testdata",
		Profile:  "prod",
		Env: []EnvPair{
			{"APP_SERVER__PORT", "8080"}, // env tops out server.port
			{"APP_DB__POOL_SIZE", "30"},  // overridden again by runtime
			{"APP_PROFILE", "prod"},      // selector, ignored as data
		},
		Vault:   vault,
		Runtime: rt,
	}
}

func demo1And3Precedence() {
	fmt.Println("\n[1+3] 6-layer precedence & provenance")
	fmt.Println(strings.Repeat("-", 68))
	res, err := Resolve(buildSources())
	must(err)
	dump(res)

	// Each key below tops out at a DIFFERENT layer — precedence proven.
	expect := []struct {
		path  Path
		layer string
	}{
		{Path{"app", "name"}, LayerDefault},
		{Path{"db", "host"}, LayerFile},
		{Path{"log", "level"}, LayerProfile},
		{Path{"server", "port"}, LayerEnv},
		{Path{"db", "password"}, LayerVault},
		{Path{"db", "pool-size"}, LayerRuntime},
		{Path{"openai", "api-key"}, LayerRuntime},
	}
	fmt.Println("\n  winning-layer assertions:")
	ok := true
	for _, e := range expect {
		got := res.SourceOf(e.path)
		mark := "ok"
		if got != e.layer {
			mark, ok = "FAIL", false
		}
		fmt.Printf("    %-20s expect %-8s got %-8s [%s]\n", joinDot(e.path), e.layer, got, mark)
	}
	fmt.Println("  result:", pass(ok))
}

// --- criterion 2: .edn and .properties resolve to the SAME map --------------

func demo2SameMap() {
	fmt.Println("\n[2] .edn and .properties -> identical nested map")
	fmt.Println(strings.Repeat("-", 68))
	ednMap, err := readEDN("testdata/equiv.edn")
	must(err)
	propMap, err := readProperties("testdata/equiv.properties")
	must(err)
	fmt.Printf("  from .edn:        %s\n", flat(ednMap))
	fmt.Printf("  from .properties: %s\n", flat(propMap))
	same := reflect.DeepEqual(ednMap, propMap)
	fmt.Println("  identical:", pass(same))
}

// --- criterion 4: DB-primary hot-reload, no restart -------------------------

func demo4HotReload() {
	fmt.Println("\n[4] runtime hot-reload (read-through cache + invalidation)")
	fmt.Println(strings.Repeat("-", 68))
	s := buildSources()
	rt := s.Runtime

	res1, _ := Resolve(s)
	v1, _ := res1.Get(Path{"db", "pool-size"})
	fmt.Printf("  before rotate: db.pool-size = %v (via %s)\n", v1, res1.SourceOf(Path{"db", "pool-size"}))

	// repeated reads should NOT re-hit the table (cache serves them)
	readsA := rt.TableReads()
	for i := 0; i < 5; i++ {
		rt.Lookup(Path{"db", "pool-size"})
	}
	readsB := rt.TableReads()
	fmt.Printf("  5 cached reads added %d table reads (read-through works)\n", readsB-readsA)

	// hot-rotate the row — no process restart
	rt.Set(Path{"db", "pool-size"}, int64(99), false, "admin:oncall")

	res2, _ := Resolve(s)
	v2, _ := res2.Get(Path{"db", "pool-size"})
	fmt.Printf("  after  rotate: db.pool-size = %v (via %s)\n", v2, res2.SourceOf(Path{"db", "pool-size"}))

	fmt.Println("\n  audit (runtime_config rows):")
	for _, r := range rt.Rows() {
		val := prettyScalar(r.Row.Value)
		if r.Row.IsSecret {
			val = mask(r.Row.Value)
		}
		fmt.Printf("    %-16s = %-14s by %-12s at %s\n",
			joinDot(r.Path), val, r.Row.UpdatedBy, r.Row.UpdatedAt.Format("15:04:05"))
	}
	fmt.Println("  result:", pass(v1.(int64) == 50 && v2.(int64) == 99 && (readsB-readsA) == 0))
}

// --- criterion 5: coercion + secret masking ---------------------------------

func demo5Coercion() {
	fmt.Println("\n[5] type coercion (durations/sizes as numbers) + secret masking")
	fmt.Println(strings.Repeat("-", 68))
	res, _ := Resolve(buildSources())

	ttl, _ := res.Get(Path{"cache", "ttl-seconds"})
	sz, _ := res.Get(Path{"upload", "max-bytes"})
	port, _ := res.Get(Path{"server", "port"}) // came from env "8080" string
	fmt.Printf("  cache.ttl-seconds  = %v (%T)\n", ttl, ttl)
	fmt.Printf("  upload.max-bytes   = %v (%T)\n", sz, sz)
	fmt.Printf("  server.port (env)  = %v (%T)  <- coerced from string\n", port, port)

	numsOK := isInt64(ttl) && isInt64(sz) && isInt64(port)

	// secrets never printed raw
	fmt.Println("  secret leaves (masked in every dump):")
	maskedOK := true
	for _, p := range []Path{{"db", "password"}, {"openai", "api-key"}} {
		key := joinDot(p)
		if !res.Secret[key] {
			maskedOK = false
		}
		raw, _ := res.Get(p)
		fmt.Printf("    %-16s = %s\n", key, mask(raw))
	}
	fmt.Println("  result:", pass(numsOK && maskedOK))
}

// --- helpers ----------------------------------------------------------------

func dump(res Resolved) {
	for _, p := range leafPaths(res.Config) {
		key := joinDot(p)
		v, _ := res.Get(p)
		val := prettyScalar(v)
		if res.Secret[key] {
			val = mask(v)
		}
		fmt.Printf("  %-20s = %-18s <- %s\n", key, val, res.Source[key])
	}
}

func flat(m Map) string {
	var parts []string
	for _, p := range leafPaths(m) {
		v, _ := getIn(m, p)
		parts = append(parts, fmt.Sprintf("%s=%s(%T)", joinDot(p), prettyScalar(v), v))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// mask reveals only shape, never the secret value (kernel: abcd...wxyz).
func mask(v any) string {
	s, ok := v.(string)
	if !ok || len(s) <= 4 {
		return "****"
	}
	return s[:2] + "…" + s[len(s)-2:] + " (secret, masked)"
}

func isInt64(v any) bool { _, ok := v.(int64); return ok }

func pass(ok bool) string {
	if ok {
		return "PASS ✓"
	}
	return "FAIL ✗"
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
