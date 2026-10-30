# docs.substreams.dev brand

GitBook takes colors, fonts, corner style, logo, icon and social preview from the site's **Customization** settings, not
from Git Sync, so they are applied by hand. This folder holds the files to upload and the values to set. It is not part
of `SUMMARY.md` and is not published as a page.

Source: `@streamingfast/ui-tokens` v0.1.0 (brand logos and `tokens.css`).

## Files

| File                                 | Use                                                                       |
|--------------------------------------|---------------------------------------------------------------------------|
| `logo-on-light.png` / `.svg`         | Header logo, light mode. Substreams by StreamingFast, 1200×308, transparent. |
| `logo-on-dark.png` / `.svg`          | Header logo, dark mode. Same, light text.                                 |
| `icon-light.png` / `.svg`            | Site icon (favicon), light mode. Substreams icon on `#fafcfc` tile, 512×512. |
| `icon-dark.png` / `.svg`             | Site icon (favicon), dark mode. Same on `#141516` tile.                   |
| `social-preview.png` / `.svg`        | Social preview (`og:image`).                                              |

PNGs are rendered from the SVGs with `rsvg-convert` (`-w 1200` for logos, `-w 512 -h 512` for icons). The icon is
the gradient mark cut out of `substreams/svg/substreams-on-dark.svg`, on the rounded tile of `streamingfast/icon/*`.

## Customization settings

Site → **Customization**. Labels match the [GitBook docs](https://gitbook.com/docs/manage-your-site/customization).

**Title, icon and logo**

- **Icon**: light mode `icon-light.png`, dark mode `icon-dark.png`.
- **Custom logo**: light mode `logo-on-light.png`, dark mode `logo-on-dark.png`.

**Themes, colors and modes**

- **Theme**: Clean.
- **Primary color**: light `#7685d7`, dark `#99a9ff`.
- **Secondary color**: light `#009a88`, dark `#5ce4cf`.
- **Tint color**: light `#fafcfc`, dark `#232729` (near-neutral tints become the page background).
- **Semantic colors** (light / dark): Info `#3c6daa` / `#7aaeef`, Success `#007c5c` / `#5bc19d`, Warning `#8c6300` /
  `#cfa354`, Danger `#a85458` / `#e88e90`.
- **Show mode toggle**: on.
- **Default mode**: Dark.

**Site styles**

- **Custom fonts** → **Add custom font** "Urbanist", regular + semibold + bold `.woff2`:
  [400](https://cdn.jsdelivr.net/fontsource/fonts/urbanist@latest/latin-400-normal.woff2),
  [600](https://cdn.jsdelivr.net/fontsource/fonts/urbanist@latest/latin-600-normal.woff2),
  [700](https://cdn.jsdelivr.net/fontsource/fonts/urbanist@latest/latin-700-normal.woff2)
  (from [Fontsource](https://fontsource.org/fonts/urbanist), OFL).
- **Font family** → **Main font**: Urbanist. **Headings**: same as main font.
- **Font family** → **Monospace**: JetBrains Mono (built-in).
- **Corner style**: Straight.

**Sharing** tab

- **Social preview**: upload `social-preview.png`.
