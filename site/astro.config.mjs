// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';

// Project GitHub Pages: https://muthuishere.github.io/cljgo
// The hand-written landing page lives at public/index.html and is served at the
// site root; Starlight owns every other route.
export default defineConfig({
	site: 'https://muthuishere.github.io',
	base: '/cljgo',
	integrations: [
		starlight({
			title: 'cljgo',
			// No right-hand "On this page" ToC — frees width for benchmark tables.
			tableOfContents: false,
			description:
				'Clojure hosted on Go: an AOT compiler that emits plain Go source, plus a tree-walk REPL. Small static binaries, fast boot, zero-binding Go interop.',
			plugins: [
				starlightLlmsTxt({
					projectName: 'cljgo',
					description:
						'Clojure hosted on Go. A compiler written in Go that AOT-emits plain Go source (the ClojureScript model), plus a tree-walk evaluator that powers the REPL and macro engine. Ships small static binaries with fast boot, zero-binding Go interop, and conformance-tested Clojure semantics verified against JVM Clojure 1.12.',
					details:
						'cljgo compiles Clojure to plain Go source and also runs it in a REPL from the same analyzer, so REPL and compiled binaries behave identically (dual-harness conformance-tested against real JVM Clojure). A compiled hello-world is a ~6.7 MB static binary that starts in ~5 ms (measured; the cljgo tool itself is ~27.5 MB). It calls any Go package with zero bindings, ships single static binaries, includes the bri app framework (config, HTTP, HTML), a dependency/publishing story on the Go module ecosystem, and core.async. Performance is honestly reported — cljgo wins the recursion/data-structure rows but loses the AOT-vs-AOT head-to-head to Glojure (6 of 8 rows); both are published. Errors are structured diagnostics with registered codes and `cljgo explain <code>` pages.',
				}),
			],
			customCss: [
				'@fontsource-variable/inter',
				'@fontsource-variable/jetbrains-mono',
				'./src/styles/theme.css',
			],
			components: {
				Footer: './src/components/Footer.astro',
			},
			editLink: {
				baseUrl: 'https://github.com/muthuishere/cljgo/edit/main/site/',
			},
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/muthuishere/cljgo' },
				{
					icon: 'comment',
					label: 'Discussions',
					href: 'https://github.com/muthuishere/cljgo/discussions',
				},
			],
			sidebar: [
				{
					label: 'Start here',
					items: [
						{ label: 'Why cljgo', slug: 'why' },
						{ label: 'Install', slug: 'install' },
						{ label: 'Quickstart', slug: 'quickstart' },
						{ label: 'FAQ', slug: 'faq' },
					],
				},
				{
					// ADR 0104 — per-language on-ramps: fundamentals-first bridges
					// for readers arriving from another language.
					label: 'Coming from…',
					items: [{ autogenerate: { directory: 'coming-from' } }],
				},
				{
					// ADR 0104 spike s67 — Rust-By-Example-style numbered course.
					// Labels are human topic names; namespaces stay inside the pages.
					label: 'By example',
					items: [{ autogenerate: { directory: 'by-example' } }],
				},
				{
					// ADR 0104 pillar 4 — which-to-use-when deciders.
					label: 'Choosing',
					items: [{ autogenerate: { directory: 'choosing' } }],
				},
				{
					label: 'Build an app — bri',
					items: [
						{ label: 'Your first app (15 min)', slug: 'bri/tutorial' },
						{ label: 'Configuration', slug: 'bri/config' },
						{ label: 'HTTP services', slug: 'bri/http' },
						{ label: 'HTML & views', slug: 'bri/html' },
						{ label: 'Security & auth', slug: 'bri/auth' },
						{ label: 'Data layer (cljg.data.cast)', slug: 'bri/db' },
						{ label: 'Tracing (bri.core.telemetry)', slug: 'bri/otel' },
					],
				},
				{
					label: 'Guides',
					items: [
						{ label: 'The resource generator', slug: 'guides/generate' },
						{ label: 'Deploy: one static binary', slug: 'guides/deploy' },
						{ label: 'Zero-binding Go interop', slug: 'guides/interop' },
						{ label: 'Concurrency & core.async', slug: 'guides/concurrency' },
						{ label: 'Dependencies & publishing', slug: 'guides/deps-publish' },
						{ label: 'Compile & ship binaries', slug: 'guides/compile' },
						{ label: 'The REPL', slug: 'guides/repl' },
						{ label: 'Dual-host `.cljc` projects', slug: 'guides/dual-host' },
					],
				},
				{
					label: 'Reference',
					items: [
						{ label: 'Compatibility', slug: 'reference/compatibility' },
						{ label: 'Benchmarks', slug: 'reference/benchmarks' },
						{ label: 'Error codes & diagnostics', slug: 'reference/diagnostics' },
						{ label: 'Architecture', slug: 'reference/architecture' },
						{ label: 'Status & roadmap', slug: 'reference/roadmap' },
						{ label: 'Releases & changelog', slug: 'reference/releases' },
					],
				},
				{
					label: 'Community',
					items: [{ label: 'Discuss & contribute', slug: 'community' }],
				},
			],
		}),
	],
});
