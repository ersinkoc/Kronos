# Kronos — BRANDING

> Time devours obsolete backups. Kronos preserves what matters.

---

## 1. Name & Mythology

**Kronos** (Κρόνος) is the leader of the Titans in Greek mythology, the son of Uranus and Gaia, and the father of Zeus. He is the personification of **devouring time** — the ancient god who swallowed his own children to prevent them from overthrowing him, and the eternal force against which all things eventually fall.

For a backup manager the metaphor is surgical:

- **Time** is the universal enemy of data. Hard drives fail, regions go down, `DROP DATABASE` runs in the wrong window, ransomware arrives at 03:00 local. Every minute that passes is a minute of drift between what you *had* and what you *have*.
- **Kronos** turns that adversarial relationship inside out. Time still devours — but Kronos decides *what*. Obsolete backups are consumed by retention; living backups are preserved outside time's reach. **Kronos is the operator's pact with time.**
- The mythological thread also maps naturally onto the product: scheduled jobs (time incarnate), retention (the devouring), point-in-time recovery (reaching back into the swallowed past and reclaiming it — recall that Zeus ultimately forces Kronos to disgorge his siblings; Kronos's own machinery is the route to undoing his work).

There is a deliberate edge to the name. Kronos is not a cuddly product. He is the oldest operator on the team, unsmiling, never missing a run, and faintly terrifying. The branding leans into that.

### 1.1 Pronunciation & Writing Conventions

- Pronounced **KRO-nos** (IPA: `/ˈkroʊ.nɒs/`). The English stress is on the first syllable; Turkish speakers say **KRO-nos** the same way.
- Always capitalised: **Kronos**. Never *kronos* in prose, never *KRONOS* in headings (reserved for the wordmark).
- The CLI binary is lowercase: `kronos`. The control-plane binary is `kronos-server`; the agent is `kronos-agent`.
- Avoid "Chronos" in user-facing copy. Kronos (Titan) and Chronos (personified time) are conflated in later Greek philosophy but we standardise on **K** to differentiate from the (many) other products named Chronos.

### 1.2 Name Clashes (acknowledged, not blockers)

- "Kronos Incorporated" is a US workforce-management company (now UKG). Different category; unlikely to cause confusion in developer/DBA communities where Kronos-the-backup-tool will be used.
- The Greek pronunciation is shared with **Cronus**; the Latinate form is **Cronos**. We pick **Kronos** (the direct Greek transliteration) because it has the strongest mythological weight and the sharpest consonant cluster visually.

### 1.3 Domain & Namespace Plan

Primary targets (to check availability):
- `kronosbackup.dev` — preferred
- `kronosdb.io` — alternative
- `kronos.sh` — alternative
- `kronosproject.org` — fallback

GitHub org: `github.com/kronosbackup` (org) containing `kronos` (the main repo).  
Go module path: `github.com/kronosbackup/kronos`.

Container registry: `ghcr.io/kronosbackup/kronos`.

Homebrew tap: `kronosbackup/kronos` → `brew install kronosbackup/kronos/kronos`.

---

## 2. Tagline System

**Primary tagline:** **"Time devours. Kronos preserves."**

Short, reversed rhythm, sets up the mythological tension, fits on a GitHub social card.

**Alternates** (use contextually):

| Tagline | Context |
|---------|---------|
| "Never forget your data." | Developer-focused, friendlier tone (hacker news, dev.to, X). |
| "Your database's pact with time." | Hero section, marketing site. |
| "Single binary. Zero dependencies. Every database." | Technical copy, comparison pages. |
| "Backups that survive the clock." | Hero variant. |
| "The oldest operator on your team." | Feature narratives, launch post. |
| "PostgreSQL, MySQL, MongoDB, Redis. One binary." | Product Hunt / aggregator listings. |
| "Schedule. Encrypt. Verify. Restore." | Four-beat sales line for slides. |

**Positioning sentence:**
> Kronos is a single, zero-dependency Go binary that performs scheduled, encrypted, deduplicated, verified backups with point-in-time recovery across PostgreSQL, MySQL, MongoDB, and Redis — with a modern WebUI, a full CLI, an MCP server, and no `pg_dump` on the target host.

**Elevator pitch (30 seconds):**
> You probably have a cron job running `pg_dump | gzip | aws s3 cp` somewhere. It has no verification, no PITR, no encryption you control, and silently failed last Tuesday. Kronos replaces all of that with one binary. It speaks the database wire protocols itself, streams encrypted deduplicated chunks to any backend, supports PITR for every database it covers, tests its own restores, and exposes a Grafana-ready dashboard and an MCP server. Install via `brew install kronos`, configure one YAML, done.

---

## 3. Visual Identity

### 3.1 Colour Palette

The palette pairs **time-weathered bronze** with a **deep void black** — antiquity meeting the digital vault. Accents are kept sparing and purposeful.

| Role | Name | Hex | RGB | Notes |
|------|------|-----|-----|-------|
| **Primary** | **Kronos Bronze** | `#B87333` | 184, 115, 51 | Main brand colour. Logo, hero CTAs. |
| **Primary dark** | **Titan Forge** | `#8B5523` | 139, 85, 35 | Hover states, pressed buttons. |
| **Primary light** | **Aged Gold** | `#D4A963` | 212, 169, 99 | Hero gradients, subtle backgrounds. |
| **Background dark** | **Void Black** | `#0B0B12` | 11, 11, 18 | Primary dark UI background. |
| **Surface dark** | **Basalt** | `#17171F` | 23, 23, 31 | Cards, elevated surfaces in dark mode. |
| **Background light** | **Parchment** | `#F5EFE4` | 245, 239, 228 | Marketing pages, light-mode canvas. |
| **Surface light** | **Ivory** | `#FBF7EF` | 251, 247, 239 | Cards on parchment. |
| **Text primary (dark UI)** | **Marble** | `#ECE7DC` | 236, 231, 220 | 93% WCAG contrast on Void Black. |
| **Text primary (light UI)** | **Ink** | `#1A1820` | 26, 24, 32 | On Parchment. |
| **Accent: Success** | **Laurel** | `#4CB07A` | 76, 176, 122 | Successful jobs, green states. |
| **Accent: Warning** | **Ember** | `#E8A93A` | 232, 169, 58 | Running jobs, caution states. |
| **Accent: Danger** | **Sacrificial Red** | `#C0352B` | 192, 53, 43 | Failures, destructive actions. |
| **Accent: Info** | **Styx Indigo** | `#4B3F72` | 75, 63, 114 | Links, info toasts. |

**Contrast verified:** every text/background pair above meets WCAG 2.1 AA (4.5:1 for body text, 3:1 for large text).

### 3.2 Typography

Three typefaces, used sparingly.

| Use | Typeface | Weights | Why |
|-----|----------|---------|-----|
| **Wordmark** | **Cinzel** (Google Fonts) | 400, 600, 900 | Roman-inscription geometry. Classical, impervious. |
| **Product UI** | **Inter** (Google Fonts, variable) | 400, 500, 600, 700 | Neutral, screen-optimised, ubiquitous. |
| **Monospace / CLI** | **JetBrains Mono** (Google Fonts) | 400, 600 | Developer comfort, excellent at small sizes. |

Fallbacks:
```css
font-family: 'Inter', system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
font-family: 'JetBrains Mono', ui-monospace, 'SF Mono', Menlo, monospace;
```

Type scale (desktop, rem):
`0.75 / 0.875 / 1 / 1.125 / 1.25 / 1.5 / 1.875 / 2.25 / 3 / 3.75`.

Line-height scale: 1.25 for headings, 1.6 for body.

### 3.3 Logo

**Concept:** an **hourglass merged with a database cylinder** (the "three stacked discs" universal DB icon), inscribed inside a broken laurel wreath. The hourglass sand is flowing *upward* — Kronos reversing entropy. One rendering shows the sand forming a faint chain of stylised "chunks" as it rises.

Forms:

1. **Primary mark** — full lockup: icon + "KRONOS" wordmark in Cinzel 900, horizontal.
2. **Stacked mark** — icon above wordmark; for favicons, docks, avatars.
3. **Icon only** — for favicons, CLI ASCII art, system tray.
4. **Monochrome mark** — single-colour renderings for dark/light variants and for print.
5. **ASCII rendering** — for the `kronos --version` banner:

```
    █  █  █▀█  █▀█  █▄ █  █▀█  █▀▀
    █▀▄  █▀▄  █ █  █ ▀█  █ █  ▀▀█
    █ █  █ █  █▄█  █  █  █▄█  ▄▄█

        Time devours. Kronos preserves.
```

Spacing: clearspace around the mark equals the height of the "K".

### 3.4 Iconography

System icons follow **Heroicons v2** style (1.5 px stroke, round caps/joins, 24 × 24 base grid), tinted to the palette. Custom Kronos icons (hourglass, chain-link, vault door, laurel) are drawn to match.

### 3.5 Imagery & Illustration

- **Hero imagery**: high-contrast renderings of bronze/stone with subtle grain. Avoid stock "data centre corridor" clichés.
- **Illustrations**: isometric, cel-shaded, warm palette. Narrative subjects: the Titan-figure as a curator of archives; hourglasses with rising sand; vaults with rotating dial locks; chain-links forming data streams.
- **Never use**: generic cloud icons, ransom-note typography, horror/skull imagery (the brand is austere, not macabre).

---

## 4. Nano Banana 2 Image Prompts

Curated prompts for generating brand imagery. Each includes aspect-ratio and style anchor tokens the model responds to well.

### 4.1 Hero Image (landing page, 16:9)

```
A dramatic cinematic scene: a monumental bronze hourglass on a polished obsidian
pedestal inside a cavernous vault. The sand inside the hourglass is flowing UPWARD
in defiance of gravity, and as the grains rise they transform into luminous
geometric chunks — each a faceted cube of amber light, stitched together by
threads of liquid gold. Behind the hourglass, tall Doric columns fade into
pitch-black depth. The floor reflects the scene faintly. A single warm spotlight
from above catches the top of the hourglass. The overall palette is rich bronze
(#B87333), aged gold (#D4A963), and deep void black (#0B0B12). Subtle volumetric
light rays, photorealistic, 8k, cinematic composition, sharp focus on the
hourglass, soft fall-off in background. Aspect ratio 16:9.
```

### 4.2 README Banner / GitHub Social Card (1200 × 630)

```
Minimalist, editorial design. Large bronze wordmark "KRONOS" in all-caps Roman
inscription style (Cinzel Black) occupies the left third, centered vertically.
To its right, a stylised hourglass icon — bronze frame, interior filled with
tessellated cube-chunks rising like sand. Beneath the wordmark, in a smaller,
restrained serif: "Time devours. Kronos preserves." Background: deep void black
(#0B0B12) with a very faint textured grain. A single horizontal hairline of
aged gold (#D4A963) runs the full width just above the tagline. Color palette:
#B87333 bronze, #D4A963 gold, #0B0B12 black, #ECE7DC marble text. Ultra-clean,
no gradients, no photography, no extra decoration. Aspect ratio 1200:630.
```

### 4.3 Technical Architecture Diagram (4:3 explainer)

```
A clean technical infographic in editorial illustration style. Top band: a
cluster of database icons — a PostgreSQL elephant silhouette, a MySQL dolphin,
a MongoDB leaf, and a Redis cube — each inside a thin bronze hexagonal frame,
connected by thin gold lines to a central node labelled "kronos-agent". The
central node is a metallic bronze shield with a small hourglass motif. A
heavier gold line rises from the shield to a higher node labelled
"kronos-server" — depicted as a taller column with a laurel wreath etched
near its base. From kronos-server, three divergent gold streams flow outward
to three cloud-shape icons labelled "S3", "Azure", "GCS", drawn as stylised
storage vaults with dial locks. Background: parchment (#F5EFE4) with very
subtle grid lines. Typography: Inter, small, precise. Palette: bronze
(#B87333), gold (#D4A963), ink (#1A1820), parchment (#F5EFE4). Flat,
two-dimensional, crisp vector feel. Aspect ratio 4:3.
```

### 4.4 Social Post — Scheduled Backups (1:1)

```
Square composition, 1:1. Center: a large bronze hourglass tipped ninety degrees
onto its side so its axis is horizontal. Inside the bulb on the left, amber
grains pile up; the bulb on the right is almost empty. Above the hourglass,
a thin curve labeled "00:00  01:00  02:00  03:00  04:00" in small JetBrains
Mono text — a timeline. A warm golden glow marks "03:00" and a small bronze
gear symbol floats just above it, indicating a scheduled job. Subtle bronze
particles drift along the timeline. Background: void black (#0B0B12) with a
barely-visible starfield. Palette: bronze, aged gold, void black, marble.
Minimal, iconographic, not illustrated in a cartoon style. Editorial, poster
quality, sharp vector feel. Aspect ratio 1:1.
```

### 4.5 Social Post — Restore / Time Reversal (1:1)

```
Square composition, 1:1. Two hourglasses face each other. The left hourglass
is fractured across its bulb, with cracks glowing faint red — a database in
distress. From the broken hourglass, a thin filament of golden sand rises,
arches over, and re-enters the top of the right hourglass, which is whole,
standing upright, its interior luminous. Between them, a small bronze emblem
of a laurel wreath surrounds the capital letter "K". Below, single line of
text in Inter 600: "Point-in-time recovery." Deep void black background with
faint aurora-like gradient in the upper third. Palette: bronze, aged gold,
deep void, a single accent of sacrificial red (#C0352B) for the crack glow.
Dramatic, editorial, iconographic. Aspect ratio 1:1.
```

### 4.6 Blog Post Cover — "Why We Don't Shell Out to pg_dump" (16:9)

```
Split-composition, 16:9. Left 40%: a monochrome sketch of a tangled bash
script on a terminal screen — the text illegible, suggesting chaos, drawn
in thin bronze-brown lines on parchment. Right 60%: a single clean bronze
cylinder — a Kronos binary — sitting upright on a polished marble plinth, a
faint golden aura around it, inscribed vertically on its face with
"kronos". A thin gold arrow flows from the messy left to the cylindrical
right, as if the script is being consumed and distilled. Palette: parchment
background (#F5EFE4), bronze (#B87333), aged gold (#D4A963), ink (#1A1820).
Editorial illustration, sharp flat vector style. Aspect ratio 16:9.
```

### 4.7 Product Hunt / Launch Hero (2:1)

```
A wide panoramic scene. In the foreground, three figures stand in a row — not
photographed, but stylised silhouettes in bronze and gold — a developer at a
terminal, an SRE with a pager, a platform engineer with a tablet. In front
of each figure, a faint bronze beam of light rises and converges on a large
central hourglass. Inside the hourglass, the sand is moving both upward and
downward simultaneously, forming a double-helix pattern of faceted golden
cubes. Above the hourglass, in Cinzel Black, the single word "KRONOS" in
warm bronze. The upper third of the image fades into a deep star-scattered
void. Palette: void black, bronze, aged gold, marble, a soft halo of Styx
indigo (#4B3F72) at the horizon line. Panoramic, cinematic, editorial.
Aspect ratio 2:1.
```

### 4.8 Feature Infographic — "Core 4 Databases" (4:5 vertical)

```
Vertical poster, 4:5. Top: the Kronos wordmark in Cinzel Black, bronze
colour. Beneath it, four numbered panels stacked vertically, each a thin
bronze-framed card on a void-black background:

Panel 1 — PostgreSQL. Silhouette of an elephant in bronze line-art. Title
"PostgreSQL 14 – 17". Three bullet captions in Inter small: "Wire protocol
native", "PITR via WAL", "Logical + physical".

Panel 2 — MySQL & MariaDB. Silhouette of a dolphin. Title "MySQL 8 • MariaDB 11".
Bullets: "GTID-aware binlog", "InnoDB consistent snapshot", "8.0 / 8.4 / 10.11 /
11.4".

Panel 3 — MongoDB. Silhouette of a stylised leaf. Title "MongoDB 6 – 8".
Bullets: "Oplog PITR", "Replica-set snapshot", "Sharded cluster".

Panel 4 — Redis. Silhouette of a cube. Title "Redis 7 • Valkey 7.2+".
Bullets: "RDB streaming", "AOF replay", "ACL preserving".

Between panels, thin gold hairlines. Bottom footer: "One binary. One
schedule. One restore." Palette: void black, bronze (#B87333), aged gold
(#D4A963), marble text (#ECE7DC). Editorial poster quality, flat vector,
extremely legible at mobile size. Aspect ratio 4:5.
```

---

## 5. Voice & Tone

### 5.1 Brand Voice Dimensions

| Dimension | Where we sit | Where we are not |
|-----------|--------------|------------------|
| Formal ↔ Casual | **Lean formal, sparingly casual** | Never jokey. |
| Technical ↔ Approachable | **Technical-first** | Not dumbed down. |
| Serious ↔ Playful | **Mostly serious with occasional dry wit** | Never zany. |
| Confident ↔ Humble | **Confident about the engineering, humble about limits** | Not arrogant. |

### 5.2 Writing Principles

- **Say the hard thing.** "Most backups aren't tested. Kronos tests them." Don't hedge.
- **Numbers over adjectives.** "400 MB/s" not "very fast". "≤ 40 MB RSS at idle" not "lightweight".
- **The operator is the hero, not Kronos.** We are a tool.
- **Never lie about the competition.** Name them, compare fairly, let the artifact speak.
- **Use mythological language deliberately, not as decoration.** "The Titan of Time" in hero copy, not scattered through API docs.
- **No emoji in technical copy.** Landing page and social posts: sparingly. Never in CLI output, logs, error messages, docs.

### 5.3 Example Copy

**Too soft:**
> Kronos is a modern, cloud-native backup solution that makes database backups easy and fun.

**On-brand:**
> Kronos is a single Go binary. It speaks the wire protocols of PostgreSQL, MySQL, MongoDB, and Redis, and it has replaced four cron scripts and an AWS SDK in the repositories where it runs.

**Too marketing-y:**
> Unleash the power of next-generation backup infrastructure.

**On-brand:**
> Point-in-time recovery, client-side encryption, deduplicated storage, and sandbox-restore verification — in one binary you can audit.

**Too jokey:**
> Oops, your DB crashed? No worries bestie, Kronos has your back!

**On-brand:**
> When the database dies at 03:17, the only useful question is: *when is our most recent verified restore point?* Kronos answers that question, always.

---

## 6. Slogan / Positioning Variants for Channels

| Channel | Variant |
|---------|---------|
| GitHub README subtitle | *Single-binary, zero-dependency database backup manager. PostgreSQL, MySQL, MongoDB, Redis. Written in pure Go.* |
| Homepage hero | **Time devours. Kronos preserves.** *Scheduled, encrypted, verified backups for PostgreSQL, MySQL, MongoDB, and Redis — in one Go binary.* |
| X / Twitter bio | *Zero-dep backup manager for PostgreSQL, MySQL, MongoDB, Redis. PITR, encrypted, deduplicated, verified. One Go binary.* |
| Product Hunt tagline | *Replace your cron backup scripts with a single Go binary.* |
| Hacker News submission | *Kronos: a zero-dependency database backup tool that speaks wire protocols natively* |
| Conference talk title | *Kronos: Why We Wrote a Pure-Go Database Backup Manager (and Deleted 40,000 Lines of Shell)* |

---

## 7. Do / Don't

**Do**
- Use "Kronos" as the product name, "kronos" as the binary name, "kronos-server" / "kronos-agent" for modes.
- Pair bronze + gold + void-black; reserve accent colours for state semantics.
- Lead with engineering substance: wire protocols, zero deps, binary size, verification.
- Use Cinzel for the wordmark only; Inter everywhere else.
- Keep the mythological thread narrative, not decorative.
- Write like the product has already shipped and is in production somewhere serious.

**Don't**
- Don't use "Chronos" (avoid conflation with other products and the philosophical term).
- Don't use blues or greens as primary (save greens for "success" state accents only).
- Don't use cartoon imagery or gradients as decoration.
- Don't personify Kronos with a mascot (no "Kronos the friendly Titan"). The brand is an austere institution, not a character.
- Don't use emoji or exclamation marks in technical docs, logs, or CLI output.
- Don't write about "backup solutions" or "cloud-native backup platforms" — that is empty enterprise cadence. Write about what the thing does.

---

## 8. Launch Assets Checklist (ship with v0.1)

- [ ] Logo SVG: horizontal lockup, stacked, icon-only, monochrome (dark + light).
- [ ] Favicon set (16/32/48/180/512, plus `.ico`, plus Apple touch icon).
- [ ] OG / Twitter card images (1200 × 630).
- [ ] GitHub repo social image.
- [ ] README hero banner.
- [ ] Landing page hero image (Nano Banana 2 §4.1).
- [ ] Architecture infographic (Nano Banana 2 §4.3).
- [ ] Core 4 databases poster (Nano Banana 2 §4.8).
- [ ] Three announcement social posts (Nano Banana 2 §4.4, §4.5, §4.7).
- [ ] Tailwind v4 `@theme` tokens with Kronos palette + Cinzel/Inter/JetBrains Mono + shadcn/ui CSS variables wired to the bronze/gold/void palette.
- [ ] Docs-site theme with light and dark variants.
- [ ] CLI banner (ASCII wordmark) and `--version` output format.
- [ ] 60-second demo video (optional, v0.1.1 OK).

---

*End of BRANDING.md*
