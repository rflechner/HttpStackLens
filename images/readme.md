# Images

## Assets

### `logo.png`
Small icon version of the logo, suitable for favicons, toolbar icons, or any context where a compact representation is needed.

### `logo-with-text.png`
Full logo including the application name, intended for splash screens, about pages, or anywhere the app name should be displayed alongside the icon.

## Optimizing images

To compress PNG files in this directory, run the following Docker command from the `images/` folder:

```sh
docker run --rm -v ${PWD}:/app -w /app alpine sh -c "apk add --no-cache pngquant jpegoptim && pngquant --ext .png --force --speed 1 *.png"
```

This uses [`pngquant`](https://pngquant.org/) for lossy PNG compression and overwrites the originals in place.

## Generating a .ico from logo.png

To generate a multi-resolution `.ico` file from `logo.png`, run the following Docker command from the `images/` folder:

```sh
docker run --rm -v ${PWD}:/app -w /app alpine sh -c "apk add --no-cache imagemagick && convert logo.png -define icon:auto-resize=256,128,64,48,32,16 logo.ico"
```

This uses [ImageMagick](https://imagemagick.org/) to produce a `logo.ico` containing multiple resolutions (16×16 to 256×256), suitable for Windows application icons.
