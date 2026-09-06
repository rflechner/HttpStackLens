# Images

## Assets

### `logo-v2.png`
Application icon used by the Wails packaging pipeline. The build tool copies it
to `build/appicon.png` and lets Wails generate the platform-specific icon.

### `splash-screen.png`
Full visual including the application name, intended for the README, splash
screens, about pages, or other branded presentation surfaces.

## Optimizing images

To compress PNG files in this directory, run the following Docker command from the `images/` folder:

```sh
docker run --rm -v ${PWD}:/app -w /app alpine sh -c "apk add --no-cache pngquant jpegoptim && pngquant --ext .png --force --speed 1 *.png"
```

This uses [`pngquant`](https://pngquant.org/) for lossy PNG compression and overwrites the originals in place.

The legacy hand-generated `logo.ico` and `go-winres` workflow are no longer
needed: Wails generates the Windows icon and resources during `app` builds.
