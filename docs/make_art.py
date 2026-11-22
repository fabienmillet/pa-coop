#!/usr/bin/env python3
"""Compose docs/banner.svg (1280x320) et docs/social.svg + social.png (1280x640) à partir de captures du jeu
(docs/art/banner-bg.jpg et docs/art/social-bg.jpg) : bandeau sombre, titre, et curseurs de joueurs façon co-op."""
import base64
import os
import subprocess

D = os.path.dirname(os.path.abspath(__file__))
FONT = "'Segoe UI', 'Helvetica Neue', Helvetica, Arial, 'Liberation Sans', sans-serif"


def img(name):
    with open(os.path.join(D, "art", name), "rb") as f:
        return "data:image/jpeg;base64," + base64.b64encode(f.read()).decode()


def cursor(x, y, name, color, s=1.0):
    """Curseur d'un autre joueur, comme le jeu les affiche en co-op (disque coloré + nom)."""
    w = len(name) * 8.2 + 16
    return f'''<g transform="translate({x} {y}) scale({s})">
    <circle r="17" fill="{color}" fill-opacity="0.6" stroke="#fff" stroke-width="2.5"/>
    <rect x="{-w / 2:.1f}" y="-9" width="{w:.1f}" height="18" rx="4" fill="#fff" fill-opacity="0.92"/>
    <text y="5" text-anchor="middle" font-family="{FONT}" font-size="13" font-weight="700" fill="#222">{name}</text>
  </g>'''


def chips(x, y, items, size, gap=12, first=True):
    out, cx = [], x
    for i, t in enumerate(items):
        w = len(t) * size * 0.56 + size * 1.4
        fill = "#f08a24" if (i == 0 and first) else "#ffffff"
        out.append(f'<rect x="{cx:.0f}" y="{y}" width="{w:.0f}" height="{size * 1.9:.0f}" rx="{size * 0.95:.0f}" fill="{fill}"/>')
        out.append(f'<text x="{cx + w / 2:.0f}" y="{y + size * 1.32:.0f}" text-anchor="middle" font-size="{size}" font-weight="700" fill="#1d2430">{t}</text>')
        cx += w + gap
    return "\n    ".join(out)


def checks(x, y, items, size, step):
    out = []
    for i, t in enumerate(items):
        yy = y + i * step
        out.append(f'<text x="{x}" y="{yy}" font-size="{size}" font-weight="800" fill="#f08a24">✓</text>')
        out.append(f'<text x="{x + size * 1.3:.0f}" y="{yy}" font-size="{size}" font-weight="600" fill="#ffffff">{t}</text>')
    return "\n    ".join(out)


def banner():
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="320" viewBox="0 0 1280 320" role="img" aria-label="pa-coop: self-hosted co-op for Prison Architect">
  <defs>
    <linearGradient id="g" x1="0" x2="1">
      <stop offset="0.30" stop-color="#11161d" stop-opacity="0"/>
      <stop offset="0.55" stop-color="#11161d" stop-opacity="0.88"/>
      <stop offset="1" stop-color="#11161d" stop-opacity="0.95"/>
    </linearGradient>
  </defs>
  <image width="1280" height="320" href="{img('banner-bg.jpg')}"/>
  <rect width="1280" height="320" fill="url(#g)"/>
  {cursor(250, 120, "Warden", "#e8457a")}
  {cursor(470, 215, "Friend", "#3fa7ff")}
  <g font-family="{FONT}">
    <text x="760" y="140" font-size="92" font-weight="800" fill="#fff" letter-spacing="-2">pa<tspan fill="#f08a24">-</tspan>coop</text>
    <text x="764" y="184" font-size="27" font-weight="600" fill="#d8dee8">Self-hosted co-op for Prison Architect</text>
    {checks(764, 226, ["Up to 16 players instead of 4", "Steam and Epic players together"], 20, 30)}
  </g>
</svg>
'''


def social():
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="640" viewBox="0 0 1280 640" role="img" aria-label="pa-coop: self-hosted co-op for Prison Architect">
  <defs>
    <linearGradient id="g" x1="0" x2="1">
      <stop offset="0.22" stop-color="#11161d" stop-opacity="0"/>
      <stop offset="0.48" stop-color="#11161d" stop-opacity="0.9"/>
      <stop offset="0.7" stop-color="#11161d" stop-opacity="0.97"/>
    </linearGradient>
  </defs>
  <image width="1280" height="640" href="{img('social-bg.jpg')}"/>
  <rect width="1280" height="640" fill="url(#g)"/>
  {cursor(250, 300, "Warden", "#e8457a", 1.2)}
  {cursor(520, 410, "Friend", "#3fa7ff", 1.2)}
  {cursor(170, 520, "Bob", "#5fd068", 1.2)}
  <g font-family="{FONT}">
    <text x="660" y="250" font-size="128" font-weight="800" fill="#fff" letter-spacing="-3">pa<tspan fill="#f08a24">-</tspan>coop</text>
    <text x="666" y="304" font-size="30" font-weight="600" fill="#d8dee8">Self-hosted co-op for Prison Architect</text>
    {checks(666, 372, ["Up to 16 players instead of 4", "Steam and Epic players together",
                       "Host from the game, or a dedicated server", "Tries to recover from desyncs"], 25, 44)}
    <text x="666" y="560" font-size="23" font-weight="600" fill="#9fb0c6">github.com/fabienmillet/pa-coop</text>
  </g>
</svg>
'''


if __name__ == "__main__":
    open(os.path.join(D, "banner.svg"), "w").write(banner())
    open(os.path.join(D, "social.svg"), "w").write(social())
    subprocess.run(["rsvg-convert", "-w", "1280", "-h", "640", os.path.join(D, "social.svg"), "-o",
                    os.path.join(D, "social.png")], check=True)
