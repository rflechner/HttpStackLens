# HttpStackLens documentation — Hugo + Docsy (POC)

A proof-of-concept documentation site where **the HTML/CSS template comes from the
[Docsy](https://www.docsy.dev/) theme** and **all the content is plain Markdown**.
Everything builds through **Docker** — you do not need Hugo, Go, Node or Git
installed on your machine. That keeps the build identical on every computer.

## How it's put together

```
Docs/docsy/
├── hugo.toml                 # site config: theme module, EN/FR languages, menus, UI
├── go.mod                    # declares the Docsy theme as a Hugo module
├── package.hugo.json         # PostCSS toolchain (merged with Docsy's npm assets)
├── Dockerfile                # production build → static site served by nginx
├── docker-compose.yml        # local dev server with live reload
├── content/
│   ├── en/                   # English — Markdown only
│   │   ├── _index.md         #   landing page (Docsy "blocks" shortcodes)
│   │   └── docs/
│   │       ├── _index.md
│   │       ├── features.md
│   │       ├── getting-started.md
│   │       ├── tutorial-upstream-proxy.md
│   │       └── tutorial-https-decrypt.md
│   └── fr/                    # French — same structure, translated
│       ├── _index.md
│       └── docs/…
└── static/images/            # logos, diagrams and screenshots
```

The Docsy theme itself is **not vendored** — it is pulled in as a Hugo module at
build time (`github.com/google/docsy/theme`). To change the look, you write
Markdown and tweak `hugo.toml`; you never touch theme HTML.

> **Why is Hugo installed via npm inside the container?** The base image
> `hugomods/hugo:debian-exts` ships Go + Node + Git, but its bundled Hugo lags
> behind the version the current Docsy theme requires (≥ 0.160). So the exact
> `hugo-extended` version is pinned via npm (`HUGO_VERSION`, default `0.164.0`) —
> the same approach the official Docsy example uses. Bump it in one place:
> `Dockerfile` / `docker-compose.yml`.

## Preview it locally (live reload)

```sh
docker compose -f Docs/docsy/docker-compose.yml up
```

Then open:

- English: <http://localhost:1313/>
- Français: <http://localhost:1313/fr/>

Use the **language dropdown** (top-right) to switch EN/FR and the search box to
search. Editing any Markdown file reloads the browser automatically. Stop with
<kbd>Ctrl</kbd>+<kbd>C</kbd>.

The first start downloads the theme + npm assets into this folder
(`node_modules/`, `resources/`, `package.json`, `go.sum` — all git-ignored); later
starts are fast.

## Build the static site (production)

```sh
# Build the image (renders Markdown → static HTML, then packages nginx)
docker build -t httpstacklens-docs Docs/docsy

# Serve it
docker run --rm -p 8080:80 httpstacklens-docs
# → http://localhost:8080
```

To get the raw static files out of the image (e.g. to publish them somewhere):

```sh
id=$(docker create httpstacklens-docs)
docker cp "$id:/usr/share/nginx/html" ./public
docker rm "$id"
```

## Editing content

- **A page** is one Markdown file under `content/<lang>/docs/`. Its `weight` in the
  front matter sets the order in the left sidebar; `linkTitle` is the short label.
- **Admonitions** use the Docsy `alert` shortcode:

  ```markdown
  {{%/* alert title="🔑 Heads up" color="info" */%}}
  Body text here.
  {{%/* /alert */%}}
  ```

  `color` is one of `info`, `warning`, `success`, `primary`.
- **Images** live in `static/images/` and are referenced from the site root, e.g.
  `![alt](/images/screenshots/request-list.png)`.
- **Keep EN and FR in sync** — same file name in `content/en/docs/` and
  `content/fr/docs/`.

## Relationship to `Docs/website/`

`Docs/website/` is the current hand-written static site. This folder is a parallel
**proof of concept** of the same content on Hugo + Docsy — it does not replace the
existing site yet.
