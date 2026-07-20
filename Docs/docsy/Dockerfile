# syntax=docker/dockerfile:1
#
# Build the HttpStackLens documentation site with Hugo + Docsy and serve the
# resulting static files with nginx. Nothing but Docker is required on the host —
# Hugo, Go, Node and Git all live inside the build image.
#
#   docker build -t httpstacklens-docs Docs/docsy
#   docker run --rm -p 8080:80 httpstacklens-docs
#   → http://localhost:8080
#
# The base image ships Go + Node + Git; the exact Hugo Extended that Docsy
# requires is pinned via npm (HUGO_VERSION), so the build stays reproducible even
# as the base image's bundled Hugo drifts.

# ── Build stage: Markdown + Docsy template → static site in /src/public ──────
FROM hugomods/hugo:debian-exts AS build
ARG HUGO_VERSION=0.164.0
WORKDIR /src

# The whole site (config, content, static assets).
COPY . .

# 1. Pin the Hugo Extended version Docsy expects (installed globally).
# 2. Pull the Docsy theme (a Hugo module).
# 3. Aggregate its npm assets (Bootstrap, FontAwesome) into package.json.
# 4. Install them + the PostCSS toolchain.
# 5. Render every Markdown page into static HTML.
RUN npm install -g --no-audit --no-fund "hugo-extended@${HUGO_VERSION}" \
 && hugo mod get github.com/google/docsy/theme \
 && hugo mod npm pack \
 && npm install --no-audit --no-fund \
 && hugo --minify --gc

# ── Runtime stage: a tiny static web server ──────────────────────────────────
FROM nginx:alpine AS serve
COPY --from=build /src/public /usr/share/nginx/html
EXPOSE 80
