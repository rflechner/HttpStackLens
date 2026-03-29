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

