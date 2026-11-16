# Themes

## Built-in themes

DashDen comes with 27 themes that everyone can pick from the theme picker, next to any presets in your config. Each person's choice is saved to their profile. A theme is more than colors: its **style** sets corner rounding, glass, glow, wallpaper, font and text size. Styles work over every page layout.

| Group | Themes |
| ----- | ------ |
| Everyday | Cozy Den, Latte, Glass, Aurora, Daylight, Midnight, Minimal, High Contrast |
| Loud | Neon Night, Neon City, Laser Red, Laser Grid, Pink Velvet, Rose Noir, Bubblegum, Retro Wave, Retro 70s, Arcade, Nuclear, Hazard, Terminal |
| Kids (bigger text, rounder, brighter) | Playroom, Rainbow, Dino, Under the Sea, Space Explorer, Unicorn |

A preset in your config with the same key as a built-in theme replaces it. To hide the built-in themes:

```yaml
theme:
  builtin-themes: false
```

### Styles for your own themes

Any preset, and the main theme, can use a style, a display name and a wallpaper:

```yaml
theme:
  presets:
    beach:
      name: Beach day
      style: glass
      wallpaper: /assets/beach.jpg
      background-color: 200 40 12
      primary-color: 190 90 60
```

| Style | Look |
| ----- | ---- |
| `classic` | DashDen's original look (the default) |
| `cozy` | Rounded cards, soft shadows, rounded font, warm glow |
| `glass` | Frosted see-through cards over a colorful wallpaper |
| `neon` | Glowing borders and titles |
| `laser` | Sharp, bright outlines over a laser grid |
| `velvet` | Very round cards with a soft pink-friendly glow |
| `retro` | Chunky offset shadows, sunset wallpaper, scanlines |
| `nuclear` | Hazard stripes and radioactive glow |
| `terminal` | Square, monospace, with scanlines |
| `soft` | Borderless cards that float on shadows |
| `flat` | Plain, borderless cards |
| `bold` | Thick, high-contrast borders |
| `kids` | Bigger text and tap targets, very round, playful shapes |

`wallpaper` takes an `http(s)://` URL or a path starting with `/`, such as a file in your assets folder.

## Den theme pack

Settings, Themes has eleven ready-made themes: Den Night, Campfire, Forest, Ocean, Candy, Paper, Sunny, High Contrast, and three seasonal ones (Spooky, Winter Holidays, Spring). Tick the ones you want and press **Add selected themes**. They're copied into `theme.presets`, so they appear in the theme picker, can be set as someone's theme on the People tab, and can be edited like any other preset.

## Live preview

In edit mode, clicking the theme picker opens the styling dialog. While you change colors or options there, the dashboard behind it changes with them, so you can see the result before saving. Cancel, or click outside the dialog, and the page goes back to how it was.

## Seasonal themes

Swap the default theme for part of the year:

```yaml
theme:
  seasonal:
    - preset: spooky
      from: "10-15"
      to: "10-31"
    - preset: winter-holidays
      from: "12-20"
      to: "01-06"   # ranges can wrap past New Year
```

Dates are month-day in the server's time zone (set `TZ` on the container), and the first matching entry wins. A theme someone picked for themselves, or one set for them on the People tab, still wins over the season, and so does choosing the default theme in the picker. Seasons can also be set up under Settings, Themes.

The themes below are community themes you can copy into `theme.presets`.

## Dark

### Teal City
![screenshot](images/themes/teal-city.png)
```yaml
theme:
  background-color: 225 14 15
  primary-color: 157 47 65
  contrast-multiplier: 1.1
```

### Midnight Navy
![screenshot](images/themes/midnight-navy.png)
```yaml
theme:
  background-color: 222 20 7
  primary-color: 215 50 60
  contrast-multiplier: 1.1
```

### Camouflage
![screenshot](images/themes/camouflage.png)
```yaml
theme:
  background-color: 186 21 20
  contrast-multiplier: 1.2
  primary-color: 97 13 80
```

### Catppuccin Frappe
![screenshot](images/themes/catppuccin-frappe.png)
```yaml
theme:
  background-color: 229 19 23
  contrast-multiplier: 1.2
  primary-color: 222 74 74
  positive-color: 96 44 68
  negative-color: 359 68 71
```

### Catppuccin Macchiato
![screenshot](images/themes/catppuccin-macchiato.png)
```yaml
theme:
  background-color: 232 23 18
  contrast-multiplier: 1.2
  primary-color: 220 83 75
  positive-color: 105 48 72
  negative-color: 351 74 73
```

### Catppuccin Mocha
![screenshot](images/themes/catppuccin-mocha.png)
```yaml
theme:
  background-color: 240 21 15
  contrast-multiplier: 1.2
  primary-color: 217 92 83
  positive-color: 115 54 76
  negative-color: 347 70 65
```

### Gruvbox Dark
![screenshot](images/themes/gruvbox.png)
```yaml
theme:
  background-color: 0 0 16
  primary-color: 43 59 81
  positive-color: 61 66 44
  negative-color: 6 96 59
```

### Kanagawa Dark
![screenshot](images/themes/kanagawa-dark.png)
```yaml
theme:
  background-color: 240 13 14
  primary-color: 51 33 68
  negative-color: 358 100 68
  contrast-multiplier: 1.2
```

### Tucan
![screenshot](images/themes/tucan.png)
```yaml
theme:
  background-color: 50 1 6
  primary-color: 24 97 58
  negative-color: 209 88 54
```

### Dracula
![screenshot](images/themes/dracula.png)
```yaml
theme:
  background-color: 231 15 21
  primary-color: 265 89 79
  contrast-multiplier: 1.2
  positive-color: 135 94 66
  negative-color: 0 100 67
```

### Shades of Purple
![screenshot](images/themes/shades-of-purple.png)
```yaml
theme:
  background-color: 243 33 25
  contrast-multiplier: 1.2
  primary-color: 50 100 49
  positive-color: 98 82 71
  negative-color: 12 77 52
```

### Neon Pink 
![screenshot](images/themes/neon-pink.png)
```yaml
theme:
  background-color: 240 27 11
  contrast-multiplier: 1.5
  primary-color: 321 100 71
  positive-color: 165 78 51
  negative-color: 360 100 71
```

### Degoog Dark
![screenshot](images/themes/degoog-dark.png)
```yaml
theme:
  background-color: 227 11 16
  contrast-multiplier: 1.2
  primary-color: 220 34 51
  positive-color: 136 53 43
  negative-color: 5 81 56
```

## Light

### Catppuccin Latte
![screenshot](images/themes/catppuccin-latte.png)
```yaml
theme:
  light: true
  background-color: 220 23 95
  contrast-multiplier: 1.0
  primary-color: 220 91 54
  positive-color: 109 58 40
  negative-color: 347 87 44
```

### Peachy
![screenshot](images/themes/peachy.png)
```yaml
theme:
  light: true
  background-color: 28 40 77
  primary-color: 155 100 20
  negative-color: 0 100 60
  contrast-multiplier: 1.1
  text-saturation-multiplier: 0.5
```

### Zebra
![screenshot](images/themes/zebra.png)
```yaml
theme:
  light: true
  background-color: 0 0 95
  primary-color: 0 0 10
  negative-color: 0 90 50
```

### Morning Paper
![screenshot](images/themes/morning-paper.png)
```yaml
theme:
  light: true
  background-color: 40 10 95
  primary-color: 25 50 35
  text-saturation-multiplier: 0.8
```

### Heather Mist
![screenshot](images/themes/heather-mist.png)
```yaml
theme:
  light: true
  background-color: 270 12 97
  primary-color: 270 35 50
```

### Degoog Light
![screenshot](images/themes/degoog-light.png)
```yaml
theme:
  light: true
  background-color: 0 0 95
  contrast-multiplier: 1.1
  primary-color: 217 36 36
  positive-color: 136 53 40
  negative-color: 5 81 56
```