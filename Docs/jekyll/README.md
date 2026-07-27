# HttpStackLens documentation (Jekyll)

A Jekyll site that pairs the **Markdown content** ported from the Docsy site
(`Docs/docsy`) with the **hand-written theme** from `Docs/website`, now expressed
as Jekyll layouts and includes. It targets the `github-pages` gem, so a local
build matches what GitHub Pages would publish.

## Structure

```
_config.yml            Site config (baseurl, kramdown/rouge, per-path lang defaults)
_data/strings.yml      Per-language nav + footer strings (keyed by page.lang)
_includes/             head, nav (with active-link + language switch), footer
_layouts/              default (skeleton) · page (doc pages with a page-head)
index.html, fr/index.html   Landing pages (hero + feature cards)
*.md, fr/*.md          Doc pages — plain Markdown with Jekyll front matter
assets/                css/ (theme) · js/ (theme toggle, mobile nav, copy buttons) · img/
```

English lives at the root, French under `/fr/`. There is no i18n plugin (GitHub
Pages would not allow one): each page carries `lang`, which drives the nav,
footer and the EN/FR switch.

## Build & preview with Docker

No local Ruby required — everything runs in the container.

Live-reload dev server on <http://localhost:4000>:

```bash
docker compose -f Docs/jekyll/docker-compose.yml up
```

One-off production build into `Docs/jekyll/_site`:

```bash
docker build -t httpstacklens-jekyll --target build Docs/jekyll
docker run --rm -v "$PWD/Docs/jekyll/_site:/site/_site" httpstacklens-jekyll
```

## Deployment

`baseurl` in `_config.yml` is empty, which suits a root or custom-domain
deployment (e.g. `codeanythingpossible.com`). For a GitHub Pages **project**
site served under `…/HttpStackLens/`, set `baseurl: "/HttpStackLens"` — every
link and asset goes through `relative_url`, so nothing else needs changing.

## Editing content

- **Doc pages** are Markdown under the root (EN) and `fr/` (FR). Front matter:
  `layout: page`, `title`, `eyebrow`, `description`, `permalink`, `lang`.
- **Nav / footer labels** live in `_data/strings.yml`, not in the templates.
- **Images** go in `assets/img/`; reference them as
  `![alt]({{ "/assets/img/…" | relative_url }})`.
- **Landing pages** (`index.html`, `fr/index.html`) are bespoke HTML on the
  `default` layout — that is the one place hero/card markup is duplicated per
  language, by design.
