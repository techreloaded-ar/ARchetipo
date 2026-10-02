#!/usr/bin/env python3
"""
render_screens.py — draws FREE-styled screen mockups as PNG files from compact JSON specs.

Usage:
    python render_screens.py screens/*.json --out screens
    python render_screens.py --check            # fonts, Pillow, one sample render
    python render_screens.py --example > screens/3-4.json

Each spec describes one screen (see references/screen-spec.md). The base state is always rendered;
every entry of "states" is rendered as a variant. Output files: <section>-<state>.png, where the
section "3.4" becomes "3-4" (e.g. 3-4-base.png, 3-4-errore.png).

The drawing follows the FREE UX Guidelines tokens of free-base.css (colors, 64 px header, 72 px menu
rail, 12-column grid with 24 px gutter, 16 px card radius, pill buttons, floating labels). It needs
Pillow only; no browser, no network. Fonts: Montserrat and Open Sans from ../assets/fonts, with a
fallback to DejaVu Sans or Pillow's built-in font when they are not found.
"""
from __future__ import annotations

import argparse
import glob
import json
import os
import sys
from copy import deepcopy
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

# ------------------------------------------------------------------ FREE tokens (free-base.css)
TEAL = "#007D8F"
TEAL_DARK = "#004652"
TEAL_DARKER = "#033540"
MINT = "#4DF3C1"
TINT_100 = "#EBF5F5"
TINT_300 = "#E1F4F5"
MUTED_1 = "#ACD7DA"
TEXT = "#404040"
TEXT_STRONG = "#2B2B43"
TEXT_TEAL = "#255A62"
TEXT_SECONDARY = "#808080"
BORDER = "#DEDEDE"
BORDER_LIGHT = "#D9D9D9"
DISABLED = "#C0C0C0"
SURFACE = "#FFFFFF"
PAGE_BG = "#F9F9F9"
READONLY_BG = "#F4F4F4"
SHADOW = "#E4ECEE"
FEEDBACK = {
    "info": ("#247AF5", "#E9F2FE"),
    "error": ("#972438", "#F5E9EB"),
    "errore": ("#972438", "#F5E9EB"),
    "warning": ("#F19D39", "#FEF5EB"),
    "success": ("#0E7D69", "#E7F2F0"),
}
STATUS = {"success": "#0E7D69", "warning": "#F19D39", "error": "#972438", "info": "#247AF5", "neutral": "#808080"}

W = 1600
MIN_H = 1000
HEADER_H = 64
TAB_BAND_H = 48
RAIL_W = 72
CONTENT_PAD = 40
GUTTER = 24
FIELD_H = 56
BTN_H = 44
ROW_H = 56
SCALE = 2  # draw at 2x and downsample for smooth edges

FONT_DIR = Path(__file__).resolve().parent.parent / "assets" / "fonts"
FONT_FILES = {
    "heading": ["Montserrat-700.ttf", "DejaVuSans-Bold.ttf"],
    "heading-semi": ["Montserrat-600.ttf", "DejaVuSans-Bold.ttf"],
    "body": ["OpenSans-400.ttf", "DejaVuSans.ttf"],
    "body-semi": ["OpenSans-600.ttf", "DejaVuSans-Bold.ttf"],
    "body-bold": ["OpenSans-700.ttf", "DejaVuSans-Bold.ttf"],
}
SYSTEM_FONT_DIRS = ["/usr/share/fonts", "/usr/local/share/fonts", "C:/Windows/Fonts", "/Library/Fonts"]


# ------------------------------------------------------------------ canvas with scale-aware drawing
class Canvas:
    def __init__(self, width: int, height: int, bg: str):
        self.img = Image.new("RGB", (width * SCALE, height * SCALE), bg)
        self.d = ImageDraw.Draw(self.img)
        self._fonts: dict = {}
        self.font_report: dict = {}

    # fonts ---------------------------------------------------------
    def font(self, key: str, size: int):
        k = (key, size)
        if k not in self._fonts:
            self._fonts[k] = self._load(key, size * SCALE)
        return self._fonts[k]

    def _load(self, key, px):
        for name in FONT_FILES[key]:
            candidates = [FONT_DIR / name] + [Path(d) / name for d in SYSTEM_FONT_DIRS]
            for c in candidates:
                if c.is_file():
                    self.font_report.setdefault(key, str(c))
                    return ImageFont.truetype(str(c), px)
            for d in SYSTEM_FONT_DIRS:
                hits = glob.glob(os.path.join(d, "**", name), recursive=True)
                if hits:
                    self.font_report.setdefault(key, hits[0])
                    return ImageFont.truetype(hits[0], px)
        self.font_report.setdefault(key, "Pillow default")
        try:
            return ImageFont.load_default(size=px)
        except TypeError:
            return ImageFont.load_default()

    # primitives ----------------------------------------------------
    def rect(self, x, y, w, h, fill=None, outline=None, width=1, radius=0):
        box = [x * SCALE, y * SCALE, (x + w) * SCALE, (y + h) * SCALE]
        if radius:
            self.d.rounded_rectangle(box, radius=radius * SCALE, fill=fill, outline=outline, width=width * SCALE)
        else:
            self.d.rectangle(box, fill=fill, outline=outline, width=width * SCALE)

    def circle(self, cx, cy, r, fill=None, outline=None, width=1):
        self.d.ellipse([(cx - r) * SCALE, (cy - r) * SCALE, (cx + r) * SCALE, (cy + r) * SCALE], fill=fill, outline=outline, width=width * SCALE)

    def line(self, x1, y1, x2, y2, fill, width=1):
        self.d.line([x1 * SCALE, y1 * SCALE, x2 * SCALE, y2 * SCALE], fill=fill, width=width * SCALE)

    def arc(self, x, y, w, h, start, end, fill, width=2):
        self.d.arc([x * SCALE, y * SCALE, (x + w) * SCALE, (y + h) * SCALE], start, end, fill=fill, width=width * SCALE)

    def text(self, x, y, s, key="body", size=14, color=TEXT, anchor="la", maxw=None):
        if s is None:
            return
        s = str(s)
        f = self.font(key, size)
        if maxw is not None:
            s = self.ellipsis(s, key, size, maxw)
        self.d.text((x * SCALE, y * SCALE), s, font=f, fill=color, anchor=anchor)

    def textlen(self, s, key="body", size=14):
        return self.d.textlength(str(s), font=self.font(key, size)) / SCALE

    def ellipsis(self, s, key, size, maxw):
        if self.textlen(s, key, size) <= maxw:
            return s
        while s and self.textlen(s + "…", key, size) > maxw:
            s = s[:-1]
        return s + "…"

    def wrap(self, s, key, size, maxw):
        words, lines, cur = str(s).split(), [], ""
        for w in words:
            trial = (cur + " " + w).strip()
            if self.textlen(trial, key, size) <= maxw or not cur:
                cur = trial
            else:
                lines.append(cur)
                cur = w
        if cur:
            lines.append(cur)
        return lines or [""]

    def overlay(self, x, y, w, h, rgba):
        layer = Image.new("RGBA", self.img.size, (0, 0, 0, 0))
        ImageDraw.Draw(layer).rectangle([x * SCALE, y * SCALE, (x + w) * SCALE, (y + h) * SCALE], fill=rgba)
        self.img = Image.alpha_composite(self.img.convert("RGBA"), layer).convert("RGB")
        self.d = ImageDraw.Draw(self.img)

    def crop(self, height):
        self.img = self.img.crop((0, 0, self.img.width, height * SCALE))
        self.d = ImageDraw.Draw(self.img)

    def save(self, path):
        out = self.img.resize((self.img.width // SCALE, self.img.height // SCALE), Image.LANCZOS)
        out.save(path, optimize=True)


# ------------------------------------------------------------------ icons (simple line glyphs)
def draw_icon(c: Canvas, name, cx, cy, color, size=22):
    s = size / 2
    lw = 2
    name = (name or "").lower()
    if name == "home":
        c.line(cx - s, cy, cx, cy - s, color, lw)
        c.line(cx, cy - s, cx + s, cy, color, lw)
        c.rect(cx - s * 0.65, cy - s * 0.1, s * 1.3, s * 1.1, outline=color, width=lw)
    elif name in ("user", "clienti", "cliente"):
        c.circle(cx, cy - s * 0.4, s * 0.4, outline=color, width=lw)
        c.arc(cx - s * 0.85, cy + s * 0.05, s * 1.7, s * 1.6, 180, 360, color, lw)
    elif name in ("doc", "pratiche", "file"):
        c.rect(cx - s * 0.65, cy - s, s * 1.3, s * 2, outline=color, width=lw, radius=2)
        for k in (-0.35, 0, 0.35):
            c.line(cx - s * 0.35, cy + s * k, cx + s * 0.35, cy + s * k, color, lw)
    elif name in ("box", "cassette", "archive"):
        c.rect(cx - s, cy - s * 0.7, s * 2, s * 1.5, outline=color, width=lw, radius=2)
        c.line(cx - s, cy - s * 0.1, cx + s, cy - s * 0.1, color, lw)
    elif name in ("list", "elenco"):
        for k in (-0.6, 0, 0.6):
            c.line(cx - s * 0.9, cy + s * k, cx + s * 0.9, cy + s * k, color, lw)
    elif name in ("search", "cerca"):
        c.circle(cx - s * 0.2, cy - s * 0.2, s * 0.6, outline=color, width=lw)
        c.line(cx + s * 0.25, cy + s * 0.25, cx + s * 0.9, cy + s * 0.9, color, lw)
    elif name in ("calendar", "calendario"):
        c.rect(cx - s, cy - s * 0.7, s * 2, s * 1.6, outline=color, width=lw, radius=2)
        c.line(cx - s, cy - s * 0.2, cx + s, cy - s * 0.2, color, lw)
    elif name in ("euro", "importi"):
        c.text(cx, cy, "€", "body-bold", size, color, anchor="mm")
    elif name in ("settings", "impostazioni"):
        c.circle(cx, cy, s * 0.85, outline=color, width=lw)
        c.circle(cx, cy, s * 0.3, outline=color, width=lw)
    elif name == "info":
        c.circle(cx, cy, s, outline=color, width=lw)
        c.text(cx, cy + 1, "i", "body-bold", int(size * 0.7), color, anchor="mm")
    elif name in ("warning", "error", "errore"):
        c.circle(cx, cy, s, outline=color, width=lw)
        c.text(cx, cy + 1, "!", "body-bold", int(size * 0.7), color, anchor="mm")
    elif name == "success":
        c.circle(cx, cy, s, outline=color, width=lw)
        c.line(cx - s * 0.45, cy, cx - s * 0.1, cy + s * 0.35, color, lw)
        c.line(cx - s * 0.1, cy + s * 0.35, cx + s * 0.5, cy - s * 0.35, color, lw)
    elif name == "empty":
        c.rect(cx - s, cy - s * 0.6, s * 2, s * 1.4, outline=color, width=lw, radius=3)
        c.line(cx - s * 0.5, cy + s * 0.1, cx + s * 0.5, cy + s * 0.1, color, lw)
    elif name == "chevron":
        c.line(cx - s * 0.45, cy - s * 0.2, cx, cy + s * 0.25, color, lw)
        c.line(cx, cy + s * 0.25, cx + s * 0.45, cy - s * 0.2, color, lw)
    elif name == "close":
        c.line(cx - s * 0.5, cy - s * 0.5, cx + s * 0.5, cy + s * 0.5, color, lw)
        c.line(cx - s * 0.5, cy + s * 0.5, cx + s * 0.5, cy - s * 0.5, color, lw)
    elif name == "edit":
        c.line(cx - s * 0.6, cy + s * 0.6, cx + s * 0.5, cy - s * 0.5, color, lw)
        c.line(cx - s * 0.6, cy + s * 0.6, cx - s * 0.6, cy + s * 0.2, color, lw)
    elif name == "trash":
        c.rect(cx - s * 0.55, cy - s * 0.4, s * 1.1, s * 1.3, outline=color, width=lw, radius=2)
        c.line(cx - s * 0.8, cy - s * 0.4, cx + s * 0.8, cy - s * 0.4, color, lw)
    elif name == "more":
        for k in (-0.5, 0, 0.5):
            c.circle(cx + s * k, cy, 2, fill=color)
    else:
        c.circle(cx, cy, s * 0.8, outline=color, width=lw)


# ------------------------------------------------------------------ components
def button(c: Canvas, x, y, label, style="primary", align="left", width=None):
    """Draws a FREE pill button; returns its width. align=right -> x is the right edge."""
    label = str(label).upper()
    tw = c.textlen(label, "body-bold", 13)
    w = width or max(150, tw + 56)
    if align == "right":
        x = x - w
    if style == "primary":
        c.rect(x, y, w, BTN_H, fill=MINT, radius=BTN_H // 2)
        color = TEAL_DARK
    elif style == "urgent":
        c.rect(x, y, w, BTN_H, fill="#E30070", radius=BTN_H // 2)
        color = SURFACE
    elif style == "disabled":
        c.rect(x, y, w, BTN_H, fill=DISABLED, radius=BTN_H // 2)
        color = SURFACE
    else:  # secondary / tertiary
        c.rect(x, y, w, BTN_H, fill=SURFACE, outline=MINT, width=2, radius=BTN_H // 2)
        color = TEAL
    c.text(x + w / 2, y + BTN_H / 2 + 1, label, "body-bold", 13, color, anchor="mm")
    return w


def icon_button(c: Canvas, cx, cy, name, color=TEAL):
    c.circle(cx, cy, 16, fill=SURFACE, outline=MINT, width=2)
    draw_icon(c, name, cx, cy, color, 16)


def field_box(c: Canvas, x, y, w, f: dict):
    """Floating-label field. Returns the height used (box + helper/error line)."""
    readonly = f.get("readonly")
    error = f.get("error")
    fill = READONLY_BG if readonly else SURFACE
    outline = FEEDBACK["error"][0] if error else (BORDER_LIGHT if readonly else BORDER)
    c.rect(x, y, w, FIELD_H, fill=fill, outline=outline, width=2 if error else 1, radius=8)
    label = str(f.get("label", ""))
    if f.get("optional"):
        label += " (Opzionale)"
    lw = c.textlen(label, "body", 12)
    c.rect(x + 10, y - 8, min(lw + 12, w - 20), 16, fill=fill)
    c.text(x + 16, y - 1, label, "body", 12, FEEDBACK["error"][0] if error else TEXT_SECONDARY, maxw=w - 28)
    control = f.get("control", "")
    inner_w = w - 28 - (28 if control in ("select", "date") else 0)
    value = f.get("value")
    if value in (None, ""):
        if f.get("placeholder"):
            c.text(x + 14, y + FIELD_H / 2 + 2, f["placeholder"], "body", 15, DISABLED, anchor="lm", maxw=inner_w)
    elif f.get("skeleton"):
        c.rect(x + 14, y + FIELD_H / 2 - 7, min(inner_w, 180), 14, fill=BORDER_LIGHT, radius=7)
    else:
        c.text(x + 14, y + FIELD_H / 2 + 2, value, "body", 15, TEXT_SECONDARY if readonly else TEXT, anchor="lm", maxw=inner_w)
    if control == "select":
        draw_icon(c, "chevron", x + w - 22, y + FIELD_H / 2, TEXT_SECONDARY, 18)
    elif control == "date":
        draw_icon(c, "calendar", x + w - 22, y + FIELD_H / 2, TEXT_SECONDARY, 16)
    extra = 0
    if error:
        c.text(x + 4, y + FIELD_H + 6, error, "body", 12, FEEDBACK["error"][0], maxw=w - 8)
        extra = 20
    elif f.get("helper"):
        c.text(x + 4, y + FIELD_H + 6, f["helper"], "body", 12, TEXT_SECONDARY, maxw=w - 8)
        extra = 20
    return FIELD_H + extra


def toggle(c: Canvas, x, y, w, f: dict):
    on = bool(f.get("value")) and str(f.get("value")).lower() not in ("no", "false", "0", "off")
    c.rect(x, y + 16, 44, 24, fill=TEAL if on else DISABLED, radius=12)
    c.circle(x + (32 if on else 12), y + 28, 9, fill=SURFACE)
    c.text(x + 56, y + 28, f.get("label", ""), "body", 15, TEXT, anchor="lm", maxw=w - 60)
    return FIELD_H


def choice_group(c: Canvas, x, y, w, f: dict):
    """radio or checkbox group: label + options on one row (wraps if needed)."""
    kind = f.get("control")
    c.text(x + 4, y - 2, f.get("label", ""), "body", 12, TEXT_SECONDARY, maxw=w - 8)
    cx, cy = x + 4, y + 30
    for opt in f.get("options", []):
        name = opt if isinstance(opt, str) else opt.get("label", "")
        checked = (not isinstance(opt, str) and opt.get("checked")) or name == f.get("value")
        tw = c.textlen(name, "body", 14)
        if cx + 28 + tw > x + w:
            cx, cy = x + 4, cy + 30
        if kind == "radio":
            c.circle(cx + 9, cy, 9, fill=SURFACE, outline=TEAL if checked else TEXT_SECONDARY, width=2)
            if checked:
                c.circle(cx + 9, cy, 4, fill=TEAL)
        else:
            c.rect(cx, cy - 9, 18, 18, fill=TEAL if checked else SURFACE, outline=TEAL if checked else TEXT_SECONDARY, width=2, radius=3)
            if checked:
                c.line(cx + 4, cy, cx + 8, cy + 4, SURFACE, 2)
                c.line(cx + 8, cy + 4, cx + 14, cy - 4, SURFACE, 2)
        c.text(cx + 26, cy, name, "body", 14, TEXT, anchor="lm")
        cx += 26 + tw + 28
    return max(FIELD_H, cy - y + 20)


def status_pill(c: Canvas, x, cy, status, label):
    color = STATUS.get(str(status).lower(), STATUS["neutral"])
    c.circle(x + 6, cy, 6, fill=color)
    c.text(x + 20, cy, label, "body", 14, TEXT, anchor="lm")


# ------------------------------------------------------------------ widgets
class Layout:
    """Measures and draws widgets inside the 12-column grid."""

    def __init__(self, c: Canvas, x0, content_w):
        self.c = c
        self.x0 = x0
        self.content_w = content_w
        self.col_w = (content_w - 11 * GUTTER) / 12

    def width_for(self, cols):
        return cols * self.col_w + (cols - 1) * GUTTER

    # measurement -----------------------------------------------------
    def fields_rows(self, widget, w):
        per_row = widget.get("per_row") or (3 if w >= 640 else 2)
        rows, cur, used = [], [], 0
        for f in widget.get("fields", []):
            span = min(int(f.get("span", 1)), per_row)
            if used + span > per_row and cur:
                rows.append(cur)
                cur, used = [], 0
            cur.append((f, span))
            used += span
        if cur:
            rows.append(cur)
        return per_row, rows

    def measure(self, widget, w):
        t = widget.get("type", "form")
        h = 24 + (30 if widget.get("title") else 0)
        inner_w = w - 48
        if widget.get("empty"):
            return h + 170 + (BTN_H + 12 if widget["empty"].get("action") else 0)
        if t == "form":
            per_row, rows = self.fields_rows(widget, w)
            for row in rows:
                h += 14 + max(self._field_h(f) for f, _ in row) + 10
            if widget.get("actions"):
                h += 12 + BTN_H
            if widget.get("lines"):
                h += 4 + 22 * len(widget["lines"])
        elif t == "table":
            n = len(widget.get("rows", []))
            h += (52 if widget.get("toolbar") else 0) + 44 + ROW_H * n + (36 if widget.get("paginator") else 0)
        elif t == "kv":
            h += 34 * len(widget.get("items", []))
        elif t == "text":
            for line in widget.get("lines", []):
                h += 22 * len(self.c.wrap(line, "body", 14, inner_w))
        elif t == "kpi":
            h += 64
        elif t == "chart":
            h += int(widget.get("height", 180))
        return h + 24

    def _field_h(self, f):
        if f.get("control") in ("radio", "checkbox"):
            return FIELD_H
        if f.get("error") or f.get("helper"):
            return FIELD_H + 20
        return FIELD_H

    # drawing ---------------------------------------------------------
    def draw(self, widget, x, y, w, h):
        c = self.c
        c.rect(x + 1, y + 3, w, h, fill=SHADOW, radius=16)
        c.rect(x, y, w, h, fill=SURFACE, radius=16)
        cy = y + 24
        if widget.get("title"):
            c.text(x + 24, cy, widget["title"], "heading-semi", 18, TEXT_TEAL, maxw=w - 48)
            cy += 30
        inner_x, inner_w = x + 24, w - 48
        if widget.get("empty"):
            e = widget["empty"]
            mid = x + w / 2
            c.circle(mid, cy + 50, 36, fill=TINT_300)
            draw_icon(c, e.get("icon", "empty"), mid, cy + 50, TEAL, 30)
            c.text(mid, cy + 108, e.get("title", "Nessun elemento"), "heading-semi", 16, TEXT_STRONG, anchor="ma")
            c.text(mid, cy + 134, e.get("text", ""), "body", 14, TEXT_SECONDARY, anchor="ma", maxw=inner_w)
            if e.get("action"):
                bw = max(150, c.textlen(str(e["action"]).upper(), "body-bold", 13) + 56)
                button(c, mid - bw / 2, cy + 170, e["action"], "secondary")
            return
        t = widget.get("type", "form")
        if t == "form":
            per_row, rows = self.fields_rows(widget, w)
            unit = (inner_w - (per_row - 1) * 16) / per_row
            for row in rows:
                cy += 14
                fx = inner_x
                row_h = max(self._field_h(f) for f, _ in row)
                for f, span in row:
                    fw = unit * span + 16 * (span - 1)
                    ctrl = f.get("control")
                    if ctrl == "toggle":
                        toggle(c, fx, cy, fw, f)
                    elif ctrl in ("radio", "checkbox"):
                        choice_group(c, fx, cy, fw, f)
                    else:
                        field_box(c, fx, cy, fw, f)
                    fx += fw + 16
                cy += row_h + 10
            if widget.get("lines"):
                cy += 4
                for line in widget["lines"]:
                    c.text(inner_x, cy, line, "body-semi", 14, TEXT_TEAL, maxw=inner_w)
                    cy += 22
            if widget.get("actions"):
                cy += 12
                bx = inner_x
                for a in widget["actions"]:
                    label = a if isinstance(a, str) else a.get("label", "")
                    style = "secondary" if isinstance(a, str) else a.get("style", "secondary")
                    bx += button(c, bx, cy, label, style) + 16
        elif t == "table":
            self._table(widget, inner_x, cy, inner_w)
        elif t == "kv":
            for k, v in widget.get("items", []):
                c.text(inner_x, cy + 8, k, "body", 14, TEXT_SECONDARY, maxw=inner_w * 0.45)
                c.text(inner_x + inner_w * 0.5, cy + 8, v, "body-semi", 14, TEXT, maxw=inner_w * 0.5)
                c.line(inner_x, cy + 33, inner_x + inner_w, cy + 33, BORDER_LIGHT, 1)
                cy += 34
        elif t == "text":
            for line in widget.get("lines", []):
                for sub in c.wrap(line, "body", 14, inner_w):
                    c.text(inner_x, cy, sub, "body", 14, TEXT)
                    cy += 22
        elif t == "kpi":
            c.text(inner_x, cy, widget.get("value", ""), "heading", 32, TEAL_DARK)
            c.text(inner_x, cy + 42, widget.get("label", ""), "body", 13, TEXT_SECONDARY, maxw=inner_w)
        elif t == "chart":
            self._chart(widget, inner_x, cy, inner_w, int(widget.get("height", 180)))

    def _table(self, widget, x, y, w):
        c = self.c
        cols = widget.get("columns", [])
        n = max(1, len(cols))
        actions = int(widget.get("row_actions", 0))
        action_w = 48 * actions + 16 if actions else 0
        col_w = (w - action_w) / n
        if widget.get("toolbar"):
            tb = widget["toolbar"] if isinstance(widget["toolbar"], dict) else {}
            c.rect(x, y + 4, 320, 40, fill=SURFACE, outline=BORDER, width=1, radius=20)
            draw_icon(c, "search", x + 22, y + 24, TEXT_SECONDARY, 16)
            c.text(x + 40, y + 24, tb.get("search", "Cerca"), "body", 14, DISABLED, anchor="lm", maxw=260)
            if tb.get("action"):
                button(c, x + w, y + 2, tb["action"], "secondary", align="right")
            y += 52
        # header
        c.line(x, y + 44, x + w, y + 44, TEAL, 2)
        for i, col in enumerate(cols):
            c.text(x + col_w * i + 8, y + 22, str(col).upper(), "body-bold", 12, TEAL_DARK, anchor="lm", maxw=col_w - 16)
        y += 44
        for r_idx, row in enumerate(widget.get("rows", [])):
            if widget.get("selected") == r_idx:
                c.rect(x, y, w, ROW_H, fill=TINT_100)
            for i in range(n):
                val = row[i] if i < len(row) else ""
                cx = x + col_w * i + 8
                if isinstance(val, dict):
                    if "status" in val:
                        status_pill(c, cx, y + ROW_H / 2, val["status"], val.get("label", ""))
                    elif val.get("skeleton"):
                        c.rect(cx, y + ROW_H / 2 - 7, col_w * 0.6, 14, fill=BORDER_LIGHT, radius=7)
                    else:
                        c.text(cx, y + ROW_H / 2, val.get("label", ""), "body", 14, TEXT, anchor="lm", maxw=col_w - 16)
                else:
                    c.text(cx, y + ROW_H / 2, val, "body", 14, TEXT, anchor="lm", maxw=col_w - 16)
            if actions:
                ax = x + w - action_w + 24
                for name in (widget.get("action_icons") or ["edit", "trash", "more"])[:actions]:
                    icon_button(c, ax, y + ROW_H / 2, name)
                    ax += 48
            c.line(x, y + ROW_H, x + w, y + ROW_H, BORDER_LIGHT, 1)
            y += ROW_H
        if widget.get("paginator"):
            pages = int(widget["paginator"]) if str(widget["paginator"]).isdigit() else 3
            px = x + w - 12
            for p in range(min(pages, 7), 0, -1):
                px -= 36
                active = p == 1
                c.rect(px, y + 8, 30, 28, fill=TEAL if active else SURFACE, outline=TEAL if active else BORDER, width=1, radius=6)
                c.text(px + 15, y + 22, str(p), "body-semi", 13, SURFACE if active else TEXT, anchor="mm")
            c.text(x, y + 22, f"1–{len(widget.get('rows', []))} di {widget.get('total', len(widget.get('rows', [])))}", "body", 13, TEXT_SECONDARY, anchor="lm")

    def _chart(self, widget, x, y, w, h):
        c = self.c
        series = widget.get("series") or [3, 5, 4, 6, 7, 5]
        labels = widget.get("labels") or [""] * len(series)
        mx = max(series) or 1
        bw = w / len(series) * 0.6
        gap = w / len(series)
        base = y + h - 24
        for i, v in enumerate(series):
            bh = (h - 40) * v / mx
            c.rect(x + i * gap + (gap - bw) / 2, base - bh, bw, bh, fill=TEAL, radius=4)
            c.text(x + i * gap + gap / 2, base + 6, labels[i] if i < len(labels) else "", "body", 12, TEXT_SECONDARY, anchor="ma")
        c.line(x, base, x + w, base, BORDER, 1)


# ------------------------------------------------------------------ page
def apply_state(base: dict, state: dict) -> dict:
    spec = deepcopy(base)
    for k, v in (state or {}).items():
        if k == "field_errors":
            for wdg in spec.get("widgets", []):
                for f in wdg.get("fields", []):
                    if f.get("label") in v:
                        f["error"] = v[f["label"]]
        elif k == "field_values":
            for wdg in spec.get("widgets", []):
                for f in wdg.get("fields", []):
                    if f.get("label") in v:
                        f["value"] = v[f["label"]]
        elif k == "empty":
            for wdg in spec.get("widgets", []):
                if wdg.get("title") == v.get("widget") or (not v.get("widget") and wdg.get("type") == "table"):
                    wdg["empty"] = v
                    break
        elif k == "widgets":
            spec["widgets"] = v
        else:
            spec[k] = v
    loading = spec.get("loading")
    if loading and (loading is True or loading.get("skeleton", True)):
        for wdg in spec.get("widgets", []):
            if loading is True or not loading.get("widget") or loading.get("widget") == wdg.get("title"):
                for f in wdg.get("fields", []):
                    if not f.get("readonly"):
                        f["skeleton"] = True
                        f["value"] = f.get("value") or "x"
                for row in wdg.get("rows", []):
                    for i in range(len(row)):
                        row[i] = {"skeleton": True}
    return spec


def render(spec: dict, out_path: str) -> dict:
    tall = 4000
    c = Canvas(W, tall, PAGE_BG)
    has_tab = bool(spec.get("context_tab"))
    top = HEADER_H + (TAB_BAND_H if has_tab else 0)
    x0 = RAIL_W + CONTENT_PAD
    content_w = W - x0 - CONTENT_PAD
    lay = Layout(c, x0, content_w)

    # shell: rail + header -----------------------------------------
    c.rect(0, 0, RAIL_W, tall, fill=TEAL)
    c.rect(0, 0, W, HEADER_H, fill=TEAL)
    c.text(20, 32, "CA", "heading", 26, SURFACE, anchor="lm")
    c.text(96, 32, spec.get("app", "Scrivania Digitale"), "heading-semi", 18, SURFACE, anchor="lm")
    user = spec.get("user", "Operatore")
    initials = "".join(p[0] for p in str(user).split()[:2]).upper()
    uw = c.textlen(user, "body", 15)
    c.circle(W - 40 - uw - 36, 32, 15, fill="#0FA4B5")
    c.text(W - 40 - uw - 36, 33, initials, "body-bold", 11, SURFACE, anchor="mm")
    c.text(W - 40, 32, user, "body", 15, SURFACE, anchor="rm")
    if has_tab:
        c.rect(0, HEADER_H, W, TAB_BAND_H, fill=TEAL)
        tab = spec["context_tab"]
        title = str(tab.get("title", "")).upper()
        tw = max(c.textlen(title, "body-bold", 13), c.textlen(tab.get("subtitle", ""), "body", 12)) + 48
        c.rect(RAIL_W + 16, HEADER_H + 6, tw, TAB_BAND_H - 6 + 16, fill=SURFACE, radius=8)
        c.rect(RAIL_W + 16, HEADER_H + TAB_BAND_H - 6, tw, 12, fill=SURFACE)
        c.text(RAIL_W + 32, HEADER_H + 18, title, "body-bold", 13, TEAL)
        c.text(RAIL_W + 32, HEADER_H + 34, tab.get("subtitle", ""), "body", 12, TEAL)
        c.rect(RAIL_W, HEADER_H + TAB_BAND_H, W - RAIL_W, 1, fill=PAGE_BG)
    # rail items
    items = spec.get("menu") or [{"label": "Home", "icon": "home"}, {"label": "Clienti", "icon": "user"}, {"label": "Pratiche", "icon": "doc", "active": True}]
    iy = HEADER_H + 24
    for it in items:
        if isinstance(it, str):
            it = {"label": it}
        active = it.get("active") or it.get("label") == spec.get("menu_active")
        if active:
            c.rect(4, iy - 4, RAIL_W - 8, 64, fill=SURFACE, radius=8)
        color = TEAL if active else SURFACE
        draw_icon(c, it.get("icon", it.get("label")), RAIL_W / 2, iy + 16, color, 22)
        c.text(RAIL_W / 2, iy + 44, it.get("label", ""), "body", 11, color, anchor="ma", maxw=RAIL_W - 8)
        iy += 76

    # page header ----------------------------------------------------
    y = top + 28
    crumbs = spec.get("breadcrumb")
    if crumbs:
        bx = x0
        for i, crumb in enumerate(crumbs):
            last = i == len(crumbs) - 1
            c.text(bx, y, crumb, "body", 13, TEXT if last else TEXT_SECONDARY)
            bx += c.textlen(crumb, "body", 13)
            if not last:
                c.text(bx + 8, y, "/", "body", 13, TEXT_SECONDARY)
                bx += 24
        y += 26
    if spec.get("back"):
        c.text(x0, y, "←  " + str(spec["back"]).upper(), "body-bold", 13, TEAL)
        y += 26
    c.text(x0, y, spec.get("title", ""), "heading", 28, TEXT_STRONG, maxw=content_w - 420)
    if spec.get("header_actions"):
        hx = x0 + content_w
        for a in reversed(spec["header_actions"]):
            label = a if isinstance(a, str) else a.get("label", "")
            style = "primary" if (not isinstance(a, str) and a.get("style") == "primary") else "secondary"
            hx -= button(c, hx, y - 4, label, style, align="right") + 12
    y += 38
    if spec.get("subtitle"):
        c.text(x0, y, spec["subtitle"], "body", 15, TEXT_SECONDARY, maxw=content_w)
        y += 26
    y += 10

    # tabs ------------------------------------------------------------
    if spec.get("tabs"):
        tx = x0
        for t in spec["tabs"]:
            label = t if isinstance(t, str) else t.get("label", "")
            active = (not isinstance(t, str) and t.get("active")) or label == spec.get("tab_active")
            tw = c.textlen(label, "body-semi", 14) + 32
            c.text(tx + 16, y + 10, label, "body-semi", 14, TEAL if active else TEXT_SECONDARY)
            if active:
                c.rect(tx, y + 34, tw, 3, fill=TEAL)
            tx += tw
        c.line(x0, y + 37, x0 + content_w, y + 37, BORDER_LIGHT, 1)
        y += 56

    # stepper ---------------------------------------------------------
    st = spec.get("stepper")
    if st and st.get("steps"):
        steps = st["steps"]
        cur = int(st.get("current", 1))
        n = len(steps)
        seg = content_w / n
        for i, label in enumerate(steps):
            cx = x0 + seg * i + seg / 2
            cy = y + 16
            done = i + 1 < cur
            current = i + 1 == cur
            if i < n - 1:
                c.line(cx + 14, cy, cx + seg - 14, cy, TEAL if done else MUTED_1, 2)
            if done or current:
                c.circle(cx, cy, 14, fill=TEAL)
                c.text(cx, cy + 1, str(i + 1), "body-bold", 13, SURFACE, anchor="mm")
            else:
                c.circle(cx, cy, 14, fill=SURFACE, outline=MUTED_1, width=2)
                c.text(cx, cy + 1, str(i + 1), "body-bold", 13, TEXT_SECONDARY, anchor="mm")
            c.text(cx, cy + 24, label, "body-bold" if current else "body", 12, TEAL_DARK if current else TEXT_SECONDARY, anchor="ma", maxw=seg - 8)
        y += 72

    # banner ----------------------------------------------------------
    b = spec.get("banner")
    if b:
        fg, bg = FEEDBACK.get(b.get("type", "info"), FEEDBACK["info"])
        lines = c.wrap(b.get("text", ""), "body", 15, content_w - 80)
        bh = max(52, 24 + 22 * len(lines))
        c.rect(x0, y, content_w, bh, fill=bg, radius=8)
        draw_icon(c, b.get("type", "info") if b.get("type") in ("success", "warning", "error", "errore") else "info", x0 + 26, y + bh / 2, fg, 20)
        for i, line in enumerate(lines):
            c.text(x0 + 50, y + 15 + 22 * i, line, "body", 15, fg)
        draw_icon(c, "close", x0 + content_w - 24, y + bh / 2, fg, 14)
        y += bh + 24

    # widgets grid ----------------------------------------------------
    widgets = spec.get("widgets", [])
    row, used = [], 0
    rows = []
    for wd in widgets:
        cols = 12 if int(wd.get("cols", 12)) > 6 else 6
        if used + cols > 12 and row:
            rows.append(row)
            row, used = [], 0
        row.append((wd, cols))
        used += cols
    if row:
        rows.append(row)
    for row in rows:
        wx = x0
        heights = [lay.measure(wd, lay.width_for(cols)) for wd, cols in row]
        rh = max(heights) if heights else 0
        for wd, cols in row:
            ww = lay.width_for(cols)
            lay.draw(wd, wx, y, ww, rh)
            wx += ww + GUTTER
        y += rh + GUTTER
    if not widgets and not spec.get("html_note"):
        c.text(x0, y, "(nessun widget nella spec)", "body", 14, TEXT_SECONDARY)
        y += 30

    # footer actions --------------------------------------------------
    acts = spec.get("actions")
    if acts:
        y += 8
        if acts.get("back"):
            button(c, x0, y, acts["back"], "secondary")
        rx = x0 + content_w
        if acts.get("primary"):
            rx -= button(c, rx, y, acts["primary"], acts.get("primary_style", "primary"), align="right") + 16
        for s in reversed(acts.get("secondary", []) or []):
            rx -= button(c, rx, y, s, "secondary", align="right") + 16
        y += BTN_H
    final_h = max(MIN_H, int(y + 36))
    c.crop(final_h)

    # overlays: loading / modal ---------------------------------------
    loading = spec.get("loading")
    if isinstance(loading, dict) and loading.get("text"):
        c.overlay(RAIL_W, top, W - RAIL_W, final_h - top, (255, 255, 255, 150))
        mx, my = RAIL_W + (W - RAIL_W) / 2, top + (final_h - top) / 2
        c.arc(mx - 22, my - 40, 44, 44, 20, 300, TEAL, 4)
        c.text(mx, my + 18, loading["text"], "body-semi", 14, TEXT, anchor="ma")
    modal = spec.get("modal")
    if modal:
        c.overlay(0, 0, W, final_h, (0, 0, 0, 191))
        mw = {"sm": 300, "lg": 800, "xl": 1140}.get(modal.get("size", ""), 600)
        body_lines = c.wrap(modal.get("text", ""), "body", 15, mw - 64)
        mh = 48 + 36 + 16 + 22 * len(body_lines) + 24 + 1 + 16 + BTN_H + 24
        mx, my = (W - mw) / 2, (final_h - mh) / 2
        c.rect(mx, my, mw, mh, fill=SURFACE, radius=16)
        draw_icon(c, "close", mx + mw - 32, my + 32, TEXT_STRONG, 16)
        c.text(mx + mw / 2, my + 48, modal.get("title", ""), "heading", 22, TEXT_STRONG, anchor="ma", maxw=mw - 96)
        ty = my + 48 + 36 + 16
        for line in body_lines:
            c.text(mx + 32, ty, line, "body", 15, TEXT)
            ty += 22
        fy = ty + 24
        c.line(mx, fy, mx + mw, fy, BORDER_LIGHT, 1)
        by = fy + 16
        if modal.get("primary"):
            button(c, mx + mw - 32, by, modal["primary"], modal.get("primary_style", "primary"), align="right")
        if modal.get("secondary"):
            button(c, mx + 32, by, modal["secondary"], "secondary")
    c.save(out_path)
    return {"path": out_path, "size": os.path.getsize(out_path), "height": final_h, "fonts": c.font_report}


# ------------------------------------------------------------------ driver
EXAMPLE = {
    "section": "3.4",
    "title": "Inserimento dati",
    "subtitle": "Mario Rossi · NDG 00451287 · Dati esemplificativi",
    "app": "Scrivania Digitale",
    "user": "Stefano Marello",
    "menu": [{"label": "Home", "icon": "home"}, {"label": "Clienti", "icon": "user"}, {"label": "Pratiche", "icon": "doc", "active": True}, {"label": "Cassette", "icon": "box"}],
    "context_tab": {"title": "Apertura contratto", "subtitle": "Cassetta di sicurezza"},
    "breadcrumb": ["Home", "Processi", "Apertura contratto"],
    "stepper": {"steps": ["Controlli", "Dati contratto", "Condizioni", "Riepilogo", "Esito"], "current": 2},
    "banner": {"type": "info", "text": "La cassetta selezionata resta prenotata fino al 31/10/2026."},
    "widgets": [
        {"type": "form", "title": "Intestatari e operatività", "cols": 6, "fields": [
            {"label": "Primo intestatario", "value": "Mario Rossi · NDG 00451287", "readonly": True, "span": 2},
            {"label": "Operatività", "value": "Disgiunta", "control": "select"},
            {"label": "Conto di addebito", "value": "IT60 X054 2811 1010 •••• 4567", "helper": "Conto intestato a Mario Rossi", "span": 3}],
         "actions": ["Aggiungi cointestatario", "Aggiungi delegati"]},
        {"type": "form", "title": "Cassetta", "cols": 6, "fields": [
            {"label": "Filiale", "value": "Parma Centro", "readonly": True},
            {"label": "Numero cassetta", "value": "A-0142"},
            {"label": "Classe / volume", "value": "C3 · 28 dm³", "readonly": True},
            {"label": "Numero chiavi", "value": "2"},
            {"label": "Durata", "value": "1 anno", "control": "select"},
            {"label": "Decorrenza", "value": "01/10/2026", "control": "date"}],
         "actions": ["Seleziona cassetta"]},
        {"type": "table", "title": "Delegati", "cols": 12, "columns": ["Nome", "Codice fiscale", "Ruolo", "Stato"], "row_actions": 2,
         "rows": [["Anna Bianchi", "BNCNNA80A41G337K", "Delegato", {"status": "success", "label": "Attivo"}],
                  ["Luca Verdi", "VRDLCU75M12G337Z", "Delegato", {"status": "warning", "label": "Da verificare"}]]},
    ],
    "actions": {"back": "Indietro", "secondary": ["Sospendi"], "primary": "Avanti"},
    "states": {
        "errore": {"banner": {"type": "error", "text": "Sono presenti errori nei dati inseriti. Correggi i campi evidenziati."},
                   "field_errors": {"Conto di addebito": "Codice IBAN formalmente errato. Verificare il valore inserito."}},
        "caricamento": {"loading": {"text": "Verifica della cassetta in corso…", "widget": "Cassetta"}},
        "vuoto": {"empty": {"widget": "Delegati", "title": "Nessun delegato", "text": "Aggiungi un delegato per consentire l'accesso alla cassetta.", "action": "Aggiungi delegato"}},
        "modale-sospensione": {"modal": {"title": "Confermi la sospensione?", "text": "La pratica resterà sospesa per 30 giorni; i dati inseriti non andranno persi.", "primary": "Conferma", "secondary": "Annulla"}},
    },
}


def section_slug(section: str) -> str:
    return str(section).replace(".", "-").replace(" ", "")


def render_spec_file(path: str, out_dir: str) -> list[dict]:
    with open(path, encoding="utf-8") as fh:
        spec = json.load(fh)
    return render_spec(spec, out_dir)


def render_spec(spec: dict, out_dir: str) -> list[dict]:
    results = []
    base = {k: v for k, v in spec.items() if k != "states"}
    slug = section_slug(spec.get("section", Path(out_dir).stem))
    os.makedirs(out_dir, exist_ok=True)
    results.append(dict(render(base, os.path.join(out_dir, f"{slug}-base.png")), state="base"))
    for name, state in (spec.get("states") or {}).items():
        merged = apply_state(base, state)
        results.append(dict(render(merged, os.path.join(out_dir, f"{slug}-{name}.png")), state=name))
    return results


def main(argv=None):
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    except (AttributeError, ValueError):
        pass
    ap = argparse.ArgumentParser(description="FREE screen specs (JSON) -> PNG")
    ap.add_argument("specs", nargs="*", help="JSON spec files (globs allowed)")
    ap.add_argument("--out", default="screens")
    ap.add_argument("--check", action="store_true", help="verify fonts and render the built-in example")
    ap.add_argument("--example", action="store_true", help="print the built-in example spec")
    args = ap.parse_args(argv)

    if args.example:
        print(json.dumps(EXAMPLE, ensure_ascii=False, indent=2))
        return 0
    if args.check:
        import PIL
        print(f"Pillow {PIL.__version__} · fonts dir: {FONT_DIR} ({'found' if FONT_DIR.is_dir() else 'MISSING'})")
        res = render_spec(EXAMPLE, args.out)
        for r in res:
            print(f"  {r['state']:<20} {os.path.basename(r['path'])}  {r['size'] / 1024:.0f} KB  {W}x{r['height']}")
        print("fonts: " + ", ".join(f"{k}={os.path.basename(v)}" for k, v in res[0]["fonts"].items()))
        print("RESULT: OK")
        return 0

    files = []
    for pattern in args.specs:
        files.extend(sorted(glob.glob(pattern)) or [pattern])
    if not files:
        ap.error("no spec files given (or use --check / --example)")
    total, errors = 0, []
    print("== render_screens report ==")
    for f in files:
        try:
            for r in render_spec_file(f, args.out):
                total += 1
                print(f"  {os.path.basename(f):<22} {r['state']:<20} -> {os.path.basename(r['path'])}  {r['size'] / 1024:.0f} KB  {W}x{r['height']}")
        except Exception as exc:  # report, do not stop the batch
            errors.append(f"{f}: {type(exc).__name__}: {exc}")
            print(f"  {os.path.basename(f):<22} ERROR {type(exc).__name__}: {exc}")
    print(f"images: {total} · spec files: {len(files)} · errors: {len(errors)}")
    print("RESULT: " + ("OK" if not errors else "CHECK FAILED"))
    return 0 if not errors else 2


if __name__ == "__main__":
    sys.exit(main())
