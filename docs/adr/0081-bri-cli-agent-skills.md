# ADR 0081 — bri.cli apps are agent-skill-native: every command is a tool an agent can drive

Date: 2026-07-24 · Status: proposed (roadmap; owner-directed: *"agent skills
inbuild like how i build winctl so that people can build stuff very easy"*).
Depends on ADR 0078 (bri.cli — the unified parameter model). Fourth of the bri.cli
block.

## Context

A modern CLI has two audiences: humans and **agents**. The owner builds tools
(winctl) that agents drive directly — the CLI *is* an agent skill: machine-readable,
self-describing, structured I/O, so an LLM can discover its commands and call them
without a human wrapper. The owner wants this **built into bri.cli** so *every*
cljgo CLI is agent-drivable for free, not something each author hand-rolls.

bri.cli is uniquely positioned for it: ADR 0078's unified parameter model means a
bri.cli app already holds a **complete, typed, described** model of its command
tree — every command, every parameter's name/type/`:about`/required/enum. That is
exactly the schema an agent needs. Agent-skill support is therefore not new
metadata to maintain; it is a **projection of the declaration the author already
wrote** (aligns with the reqsume-kernel agent-skills layer).

## Decision

### 1. Structured output everywhere — `--json`

A global `--json` flag makes every command emit its result as JSON on stdout
(human-formatted text otherwise), and errors as a structured `diag` envelope (ADR
0015) rather than prose. Non-interactive by construction: `--json` implies no
prompts (all params must come from flags/env — the agent path of ADR 0078's
resolution order). So an agent invokes any command and parses the result; a human
reads the pretty form.

### 2. Self-description — `<app> skills` / `<app> --schema`

From the command tree, bri.cli emits a machine-readable **skill manifest**: each
command as a tool with its parameters as a typed input schema (name, type,
description, required, enum) — JSON-Schema-shaped, so it drops straight into an
agent's tool list. Two blessed renderings from the one model:

- **`--schema`** — a JSON tool list (MCP-tool / function-calling shape).
- **`skills` / `cljgo generate skill`** — a `SKILL.md` agent-skill file (the
  winctl pattern generalized): the frontmatter + command reference an agent reads
  to learn the tool, generated from the same declarations and kept in sync.

Because it is generated from the live command tree, the manifest can never drift
from the actual CLI — the failure mode hand-written skill docs always hit.

### 3. Run as an agent tool server — `<app> mcp` (opt-in)

Since the schema and the dispatch already exist, a bri.cli app can optionally run
as an **MCP stdio server** (`<app> mcp`): it advertises each command as a tool and
executes calls through the same handlers the CLI uses — one implementation, two
front doors (human CLI + agent tool server). This is opt-in depth; the `--json` +
`skills` surface covers the common "an agent shells out to the binary" case with
no server.

**LLM/tool orchestration uses toolnexus (owner directive, 2026-07-24).** The
mirror of "expose the CLI *to* an agent" is a bri.cli app that itself *calls* an
LLM or orchestrates tools (an AI-assisted command, or an eventual `bri.ai`/`bri.llm`
battery under ADR 0075). For any such LLM-first need the blessed Go library is the
owner's **toolnexus**, not a third-party LLM SDK — subject to the same pure-Go /
`CGO_ENABLED=0` gate every bri dependency passes (the static-binary + `cljgo dist`
guarantee).

### 4. Scaffolding — agent-ready by default

`cljgo new --template cli` generates a CLI that already prints `--json`, answers
`skills`, and ships a `SKILL.md` — so "people can build stuff very easy" is the
default experience, not an advanced mode. An author adds a command with its
params (ADR 0078) and it is *immediately* an agent-callable tool with a correct
schema.

## Consequences

- Every bri.cli app is dual-audience for free: a human runs it interactively, an
  agent discovers its commands via `--schema`/`SKILL.md` and calls them with
  `--json`, and (opt-in) drives it as an MCP server — all from the one parameter
  declaration, nothing extra to write or maintain.
- Agent-skill metadata cannot rot: it is a projection of the command tree, not a
  parallel document.
- The unified model (0078) + agent-skills (0081) together are the payoff: declare
  a command once → get a scriptable CLI, a friendly interactive UI, generated help,
  a typed agent schema, and an MCP tool. This is the "build stuff very easy" the
  owner means, and it makes cljgo a natural language for writing the small tools
  agents run.
- Roadmap ADR: ratifies the shape (`--json` everywhere, `--schema`/`SKILL.md` from
  the tree, opt-in `mcp` server, agent-ready scaffolding); the manifest schema,
  the MCP server, and the generator land on their own spec/gates.
- Not chosen: a separate skill-definition file the author maintains by hand (drift,
  and it defeats "for free"); forcing an MCP server on every CLI (the `--json` +
  manifest path is lighter and covers most agents); a bespoke schema format (JSON
  Schema / MCP tool shape is what agents already consume).
