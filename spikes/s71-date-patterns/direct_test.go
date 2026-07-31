package pattern

import "testing"

// The same 4,000-pattern corpus, against the design that emits no Go layout.
func TestDirectDifferentialAgainstJVM(t *testing.T) {
	oracle := loadOracle(t)
	var agree, refused, jvmRejected, wrongAccept int
	diverge := []string{}
	for p, want := range oracle {
		if want == "!ERR" {
			jvmRejected++
			if _, err := CompileDirect(p); err == nil {
				wrongAccept++
			}
			continue
		}
		d, err := CompileDirect(p)
		if err != nil {
			refused++
			continue
		}
		if got := d.Format(ref); got != want {
			if len(diverge) < 10 {
				diverge = append(diverge, p+"\n      got  "+got+"\n      want "+want)
			}
		} else {
			agree++
		}
	}
	t.Logf("DIRECT: corpus=%d agree=%d refused=%d jvm-rejected=%d wrongly-accepted=%d DIVERGE=%d",
		len(oracle), agree, refused, jvmRejected, wrongAccept, len(diverge))
	for _, d := range diverge {
		t.Errorf("DIVERGENCE: %s", d)
	}
}

func BenchmarkDirectFormat(b *testing.B) {
	d, err := CompileDirect(benchPattern)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = d.Format(ref)
	}
}

func BenchmarkDirectCompile(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := CompileDirect(benchPattern); err != nil {
			b.Fatal(err)
		}
	}
}
