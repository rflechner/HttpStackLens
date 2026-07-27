# syntax=docker/dockerfile:1
#
# Build / serve the HttpStackLens Jekyll documentation site. Ruby, Bundler and
# the github-pages gem set all live in the image, so the site builds identically
# on any machine with Docker — no local Ruby needed.
#
#   Build the static site into ./_site:
#     docker build -t httpstacklens-jekyll --target build Docs/jekyll
#     docker run --rm -v "$PWD/Docs/jekyll/_site:/site/_site" httpstacklens-jekyll
#
#   Or, simpler, use docker-compose for a live-reload dev server:
#     docker compose -f Docs/jekyll/docker-compose.yml up
#     → http://localhost:4000
#
# Ruby 3.3 matches the GitHub Pages build environment closely.
FROM ruby:3.3-slim AS build

# github-pages' native gems (ffi, eventmachine, sass) need a compiler toolchain.
RUN apt-get update \
 && apt-get install -y --no-install-recommends build-essential git \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /site

# Install gems first so this layer caches until the Gemfile changes.
COPY Gemfile ./
RUN bundle install

COPY . .

# Default: a production build into /site/_site (mount it out to collect it).
ENV JEKYLL_ENV=production
CMD ["bundle", "exec", "jekyll", "build", "--destination", "_site"]
