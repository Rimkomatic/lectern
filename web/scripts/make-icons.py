"""Draws the app icons (the picture on a phone's home screen) from public/favicon.svg.

Run it when favicon.svg changes; the PNG files it writes are committed, so
building Lectern never needs this. It uses headless Chrome through Playwright:

    pip install playwright
    E2E_CHROME=/usr/bin/google-chrome python3 web/scripts/make-icons.py
    (without E2E_CHROME it uses the browser from `playwright install chromium`)

Writes web/public/icons/:
  icon-192.png, icon-512.png   the rounded icon, transparent corners
  maskable-512.png             full-bleed, for Android, which cuts its own shape
  apple-touch-icon.png         full-bleed 180x180, for iOS, which rounds it itself
"""
import base64
import os
import pathlib
import sys

from playwright.sync_api import sync_playwright

ROOT = pathlib.Path(__file__).resolve().parent.parent
SRC = (ROOT / "public" / "favicon.svg").read_text()
OUT = ROOT / "public" / "icons"

ROUNDED_RECT = '<rect width="32" height="32" rx="10" fill="#f4a720"/>'
if ROUNDED_RECT not in SRC:
    sys.exit("favicon.svg no longer starts with the rounded amber square this script expects")


def full_bleed(scale):
    """The same picture on a square that fills the whole canvas, shrunk a little
    so that nothing important sits where a circular mask would cut."""
    glyph = SRC.replace(ROUNDED_RECT, "")
    glyph = glyph[glyph.index(">") + 1 : glyph.rindex("</svg>")]
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">'
        '<rect width="32" height="32" fill="#f4a720"/>'
        f'<g transform="translate(16 16) scale({scale}) translate(-16 -16)">{glyph}</g></svg>'
    )


ICONS = [
    ("icon-192.png", 192, SRC),
    ("icon-512.png", 512, SRC),
    ("maskable-512.png", 512, full_bleed(0.92)),
    ("apple-touch-icon.png", 180, full_bleed(1.0)),
]


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    with sync_playwright() as p:
        chrome = os.environ.get("E2E_CHROME")
        browser = p.chromium.launch(executable_path=chrome, args=["--no-sandbox"]) if chrome else p.chromium.launch()
        for name, size, svg in ICONS:
            page = browser.new_page(viewport={"width": size, "height": size})
            data = base64.b64encode(svg.encode()).decode()
            page.set_content(
                '<body style="margin:0;background:transparent">'
                f'<img src="data:image/svg+xml;base64,{data}" width="{size}" height="{size}" style="display:block">'
            )
            page.wait_for_function("document.images[0].complete")
            page.screenshot(path=str(OUT / name), omit_background=True)
            page.close()
            print("wrote", OUT / name)
        browser.close()


if __name__ == "__main__":
    main()
