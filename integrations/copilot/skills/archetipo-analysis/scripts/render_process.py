#!/usr/bin/env python3
"""
render_process.py — draws the swimlane view of a process as a PNG file from a compact JSON spec.

Usage:
    python render_process.py processo.json --out screens
    python render_process.py --check --out check      # fonts, Pillow, one sample render
    python render_process.py --example > processo.json

The notation is the customer's swimlane notation (see references/process-spec.md):
    - phases ("blocchi funzionali") are the columns, drawn as a chevron bar;
    - lanes are the users who own the steps (or a lane of systems with no user), with a tag on the left;
    - nodes are the applications the user works in, coloured by technology (web, 3270, 3270 with
      webisation planned, application not queried by the user), with a dashed border when the step is
      optional; plus decisions, mail notifications, the start and the closures OK/KO;
    - flows carry the trigger of the event and its ID; a solid line is a passage between integrated
      applications, a dashed line a passage the user makes by hand; the colour says whether the
      process moves forward, goes back to a previous phase or is cancelled;
    - badges mark a step handled with a modal (M) and a planned or ongoing evolution (gear).
Optional phase notes are drawn as boxes under the lanes. The legend lists only what the figure uses.
The output file is swimlane-<id>.png (swimlane-processo.png by default); build_docx.py puts every
image whose name starts with "swimlane" on a landscape page.

Layout is deterministic: every node occupies a slot of its phase column, flows run only in the free
gaps between slots and in the free corridors between lanes, so a line never crosses a node.
Parallel lines get separate tracks. Needs Pillow only; the fonts and the drawing canvas are those
of render_screens.py.

The report ends with RESULT: OK or RESULT: CHECK FAILED. It fails on an invalid spec and on a
label that does not fit its box (shorten it); overlapping flow labels and a text that would be
smaller than 6 pt on a landscape A4 page are reported as warnings.
"""
from __future__ import annotations

import argparse
import glob
import json
import math
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from render_screens import (  # noqa: E402  (same folder, imported after the path is set)
    BORDER,
    MUTED_1,
    PAGE_BG,
    SCALE,
    SURFACE,
    TEAL,
    TEAL_DARK,
    TEAL_DARKER,
    TEXT,
    TEXT_SECONDARY,
    TEXT_STRONG,
    TINT_300,
    Canvas,
)

# ------------------------------------------------------------------ geometry
MARGIN = 28
LANE_HEADER_W = 140
NODE_W = 126
NODE_H = 74
PILL_H = 52
DIAMOND_H = 86
MAIL_W, MAIL_H = 64, 42
BAND_H = 114  # height of the node band of a lane
GAP_MIN = 40
GAP_PAD = 14
TRACK_SPACING = 10
TRACK_H = 12
CORRIDOR_MIN = 18
PHASE_BAR_H = 44
CHEVRON_TIP = 14
PORT_SPACING = 12
ARROW = 10

NODE_FONT = 16
LABEL_FONT = 14
LABEL_MAX_W = 140
GAP_LABEL_MAX_W = 96  # labels that sit in the gap between two adjacent nodes wrap sooner
LABEL_LINE_H = 18
NOTE_FONT = 14
LEGEND_FONT = 13
LEGEND_ROW_H = 24
LEGEND_ROWS = 5
LANDSCAPE_IMAGE_CM = 25.7  # A4 landscape with 2 cm margins (same values as build_docx.py)
LANDSCAPE_IMAGE_MAX_HEIGHT_CM = 14.5
MIN_READABLE_PT = 6.0

# ------------------------------------------------------------------ notation
FLOW_KINDS = {
    "avanzamento": (TEAL, "Avanzamento del processo"),
    "ricircolo": ("#5E9732", "Ritorno a una fase precedente"),
    "annullo": ("#E0702E", "Annullo o chiusura anticipata"),
}
APP_BORDER = "#002060"
APP_TECH = {
    "web": ("#92D050", "Applicazione web"),
    "3270": ("#D9D9D9", "Applicazione 3270"),
    "3270-webizzazione": ("#FFC000", "Applicazione 3270 con webizzazione pianificata"),
    "sistema": ("#9DC3E6", "Applicazione non interrogata dall'utente (solo passaggio di informazioni)"),
    "da-definire": ("#FFFFFF", "Tecnologia dell'applicazione da definire"),
}
MAIL_RECIPIENTS = {
    "utente": ("#1F7A1F", "Notifica mail a un utente"),
    "pool": ("#E0B000", "Notifica mail a un pool di risorse"),
}
CLOSURE = {"ok": ("#2E9E44", "Chiusura processo OK"), "ko": ("#D93025", "Chiusura processo KO")}
NODE_KINDS = ("applicazione", "decisione", "notifica", "avvio", "chiusura")
LANE_KINDS = ("utente", "sistemi")
USER_TAG = "#FEB811"
SYSTEMS_TAG = "#E3E7E8"
FIRST_RELEASE_RING = "#F28C28"
MODAL_BADGE = "#2E4C60"
EVENT_BORDER = "#9AA5A8"
GEAR = "#8C9599"
UNINTEGRATED = "#5F6B6E"


class SpecError(Exception):
    pass


# ------------------------------------------------------------------ model
def build_model(spec: dict) -> dict:
    phases = spec.get("phases") or []
    lanes = spec.get("lanes") or []
    nodes = spec.get("nodes") or []
    flows = spec.get("flows") or []
    errors = []
    if not phases:
        errors.append("no phases")
    if not lanes:
        errors.append("no lanes")
    if not nodes:
        errors.append("no nodes")
    phase_idx = {p.get("id"): i for i, p in enumerate(phases)}
    lane_idx = {lane.get("id"): i for i, lane in enumerate(lanes)}
    if len(phase_idx) != len(phases) or None in phase_idx:
        errors.append("every phase needs a distinct id")
    if len(lane_idx) != len(lanes) or None in lane_idx:
        errors.append("every lane needs a distinct id")
    for lane in lanes:
        if lane.get("kind", "utente") not in LANE_KINDS:
            errors.append(f"lane {lane.get('id')}: kind must be one of {', '.join(LANE_KINDS)}")

    by_id, cells = {}, {}
    for n in nodes:
        nid = n.get("id")
        if not nid or nid in by_id:
            errors.append(f"node {nid!r}: missing or duplicated id")
            continue
        if n.get("lane") not in lane_idx:
            errors.append(f"node {nid}: unknown lane {n.get('lane')!r}")
            continue
        if n.get("phase") not in phase_idx:
            errors.append(f"node {nid}: unknown phase {n.get('phase')!r}")
            continue
        kind = n.get("kind", "applicazione")
        if kind not in NODE_KINDS:
            errors.append(f"node {nid}: kind must be one of {', '.join(NODE_KINDS)}")
            continue
        if kind == "applicazione" and n.get("tecnologia", "web") not in APP_TECH:
            errors.append(f"node {nid}: tecnologia must be one of {', '.join(APP_TECH)}")
            continue
        if kind == "notifica" and n.get("destinatario", "utente") not in MAIL_RECIPIENTS:
            errors.append(f"node {nid}: destinatario must be one of {', '.join(MAIL_RECIPIENTS)}")
            continue
        if kind == "chiusura" and n.get("esito", "ok") not in CLOSURE:
            errors.append(f"node {nid}: esito must be one of {', '.join(CLOSURE)}")
            continue
        if not str(n.get("label", "")).strip() and kind != "chiusura":
            errors.append(f"node {nid}: empty label")
        node = dict(n, kind=kind, r=lane_idx[n["lane"]], p=phase_idx[n["phase"]])
        if kind == "applicazione":
            node["tecnologia"] = n.get("tecnologia", "web")
        if kind == "notifica":
            node["destinatario"] = n.get("destinatario", "utente")
        if kind == "chiusura":
            node["esito"] = n.get("esito", "ok")
            node["label"] = str(n.get("label") or CLOSURE[node["esito"]][1])
        by_id[nid] = node
        cells.setdefault((node["r"], node["p"]), []).append(node)

    # slots: explicit "slot" first, then the others fill the first free slot, in list order
    for (r, p), members in cells.items():
        taken = {}
        for node in members:
            if "slot" in node:
                s = int(node["slot"])
                if s < 0 or s in taken:
                    errors.append(f"node {node['id']}: slot {s} already used in lane {lanes[r]['id']}, phase {phases[p]['id']}")
                taken[s] = node
        free = 0
        for node in members:
            if "slot" not in node:
                while free in taken:
                    free += 1
                node["slot"] = free
                taken[free] = node
    slots = [1] * len(phases)
    for node in by_id.values():
        slots[node["p"]] = max(slots[node["p"]], int(node["slot"]) + 1)
    offset, acc = [], 0
    for s in slots:
        offset.append(acc)
        acc += s
    for node in by_id.values():
        node["pos"] = offset[node["p"]] + int(node["slot"])

    checked_flows = []
    for i, f in enumerate(flows):
        s, t = by_id.get(f.get("from")), by_id.get(f.get("to"))
        if s is None or t is None:
            errors.append(f"flow {i + 1}: unknown node {f.get('from')!r} or {f.get('to')!r}")
            continue
        if s is t:
            errors.append(f"flow {i + 1}: a flow cannot start and end on the same node ({s['id']})")
            continue
        kind = f.get("kind", "avanzamento")
        if kind not in FLOW_KINDS:
            errors.append(f"flow {i + 1}: kind must be one of {', '.join(FLOW_KINDS)}")
            continue
        route = f.get("route", "auto")
        if route not in ("auto", "sotto"):
            errors.append(f"flow {i + 1}: route must be auto or sotto")
            continue
        integrated = f.get("integrato", True)
        if not isinstance(integrated, bool):
            errors.append(f"flow {i + 1}: integrato must be true or false")
            continue
        checked_flows.append(
            dict(
                f,
                i=i,
                s=s,
                t=t,
                kind=kind,
                route=route,
                integrato=integrated,
                label=str(f.get("label", "")).strip(),
                evento=str(f.get("evento", "")).strip(),
            )
        )

    if errors:
        raise SpecError("; ".join(errors))
    warnings = []
    used_lanes = {n["r"] for n in by_id.values()}
    for r, lane in enumerate(lanes):
        if r not in used_lanes:
            warnings.append(f"lane {lane['id']} has no node")
    return {
        "phases": phases,
        "lanes": lanes,
        "nodes": by_id,
        "flows": checked_flows,
        "slots": slots,
        "offset": offset,
        "positions": acc,
        "warnings": warnings,
    }


# ------------------------------------------------------------------ routing on the grid
def assign_tracks(intervals):
    """intervals: list of (lo, hi, key). Returns {key: track} and the number of tracks."""
    ends, out = [], {}
    for lo, hi, key in sorted(intervals, key=lambda x: (x[0], x[1])):
        for k, end in enumerate(ends):
            if end < lo:
                ends[k] = hi
                out[key] = k
                break
        else:
            ends.append(hi)
            out[key] = len(ends) - 1
    return out, len(ends)


def plan_routes(m: dict):
    """Topological routing: which gaps, which corridor, which side. No pixel yet."""
    lanes_n = len(m["lanes"])
    occupied = {(n["r"], n["pos"]) for n in m["nodes"].values()}
    for f in m["flows"]:
        s, t = f["s"], f["t"]
        rs, ps, rt, pt = s["r"], s["pos"], t["r"], t["pos"]
        if rs == rt and pt > ps and (f["route"] == "auto" or pt == ps + 1) and not any(
            (rs, q) in occupied for q in range(ps + 1, pt)
        ):
            f["mode"] = "straight"
            f["exit"], f["gap_exit"], f["gap_entry"] = "right", ps + 1, pt
            continue
        f["gap_entry"] = pt
        if pt == ps and not any((q, ps) in occupied for q in range(min(rs, rt) + 1, max(rs, rt))):
            # same column, nothing in between: a straight vertical arrow from one node to the other
            f["mode"], f["gap_exit"], f["gap_entry"] = "column", None, None
            f["exit"], f["entry"] = ("bottom", "top") if rt > rs else ("top", "bottom")
            continue
        if pt < ps:
            # a flow going back loops through the corridor right above (or below) its source
            c = rs if (rt <= rs and f["route"] == "auto") else rs + 1
            f["mode"], f["corridor"], f["gap_exit"] = "loop", min(c, lanes_n), None
            f["exit"] = "top" if c == rs else "bottom"
            continue
        if pt > ps:
            f["exit"], f["gap_exit"] = "right", ps + 1
        else:
            f["exit"], f["gap_exit"] = "left", ps
        if f["gap_exit"] == f["gap_entry"]:
            f["mode"] = "vertical"
            continue
        f["mode"] = "corridor"
        if f["route"] == "sotto":
            c = max(rs, rt) + 1
        elif rs == rt:
            c = rs
        elif rt > rs:
            c = rt
        else:
            c = rs
        f["corridor"] = min(c, lanes_n)

    # vertical segments per gap, horizontal segments per corridor (levels: lane r = 2r+1, corridor c = 2c)
    gap_segments, corridor_segments = {}, {}
    for f in m["flows"]:
        rs, rt = f["s"]["r"], f["t"]["r"]
        if f["mode"] == "vertical":
            lo, hi = sorted((2 * rs + 1, 2 * rt + 1))
            gap_segments.setdefault(f["gap_exit"], []).append((lo, hi, (f["i"], "v")))
        elif f["mode"] == "corridor":
            c = f["corridor"]
            lo, hi = sorted((2 * rs + 1, 2 * c))
            gap_segments.setdefault(f["gap_exit"], []).append((lo, hi, (f["i"], "v1")))
            lo, hi = sorted((2 * c, 2 * rt + 1))
            gap_segments.setdefault(f["gap_entry"], []).append((lo, hi, (f["i"], "v2")))
            lo, hi = sorted((f["gap_exit"], f["gap_entry"]))
            corridor_segments.setdefault(c, []).append((lo, hi, f["i"]))
        elif f["mode"] == "loop":
            c = f["corridor"]
            lo, hi = sorted((2 * c, 2 * rt + 1))
            gap_segments.setdefault(f["gap_entry"], []).append((lo, hi, (f["i"], "v2")))
            corridor_segments.setdefault(c, []).append((f["gap_entry"], f["s"]["pos"] + 0.5, f["i"]))
    gap_tracks, gap_count = {}, {}
    for g, segs in gap_segments.items():
        gap_tracks[g], gap_count[g] = assign_tracks(segs)
    corridor_tracks, corridor_count = {}, {}
    for c, segs in corridor_segments.items():
        corridor_tracks[c], corridor_count[c] = assign_tracks(segs)
    return gap_tracks, gap_count, corridor_tracks, corridor_count


# ------------------------------------------------------------------ drawing helpers
def polygon(c: Canvas, pts, fill=None, outline=None, width=1):
    c.d.polygon([(x * SCALE, y * SCALE) for x, y in pts], fill=fill, outline=outline, width=int(width * SCALE))


def polyline(c: Canvas, pts, color, width=2, dashed=False):
    for (x1, y1), (x2, y2) in zip(pts, pts[1:]):
        if not dashed:
            c.line(x1, y1, x2, y2, color, width)
            continue
        length = math.hypot(x2 - x1, y2 - y1)
        if length == 0:
            continue
        dash, gap, d = 7, 5, 0.0
        while d < length:
            e = min(d + dash, length)
            ax, ay = x1 + (x2 - x1) * d / length, y1 + (y2 - y1) * d / length
            bx, by = x1 + (x2 - x1) * e / length, y1 + (y2 - y1) * e / length
            c.line(ax, ay, bx, by, color, width)
            d = e + gap
    if not dashed:  # round joints, so the elbows look continuous
        for x, y in pts[1:-1]:
            c.circle(x, y, width / 2, fill=color)


def dashed_rect(c: Canvas, x0, y0, x1, y1, color, width=2):
    polyline(c, [(x0, y0), (x1, y0), (x1, y1), (x0, y1), (x0, y0)], color, width, dashed=True)


def arrow_head(c: Canvas, tip, prev, color):
    tx, ty = tip
    px, py = prev
    if abs(tx - px) >= abs(ty - py):
        d = 1 if tx >= px else -1
        pts = [(tx, ty), (tx - d * ARROW, ty - ARROW / 2), (tx - d * ARROW, ty + ARROW / 2)]
    else:
        d = 1 if ty >= py else -1
        pts = [(tx, ty), (tx - ARROW / 2, ty - d * ARROW), (tx + ARROW / 2, ty - d * ARROW)]
    polygon(c, pts, fill=color)


def dashed_vline(c: Canvas, x, y1, y2, color):
    y = y1
    while y < y2:
        c.line(x, y, x, min(y + 6, y2), color, 1)
        y += 11


def overlaps(a, b, pad=2):
    return not (a[2] + pad <= b[0] or b[2] + pad <= a[0] or a[3] + pad <= b[1] or b[3] + pad <= a[1])


def draw_envelope(c: Canvas, x0, y0, w, h, color):
    c.rect(x0, y0, w, h, fill=SURFACE, outline=color, width=2)
    c.line(x0, y0, x0 + w / 2, y0 + h * 0.55, color, 2)
    c.line(x0 + w, y0, x0 + w / 2, y0 + h * 0.55, color, 2)
    c.text(x0 + w / 2, y0 + h * 0.36, "@", "body-bold", int(h * 0.45), color, anchor="mm")


def draw_gear(c: Canvas, cx, cy, r=10):
    for k in range(8):
        a = k * math.pi / 4
        c.circle(cx + math.cos(a) * r, cy + math.sin(a) * r, r * 0.32, fill=GEAR)
    c.circle(cx, cy, r * 0.85, fill=GEAR)
    c.circle(cx, cy, r * 0.38, fill=SURFACE)


def draw_modal_badge(c: Canvas, cx, cy, r=11):
    c.circle(cx, cy, r, fill=MODAL_BADGE, outline=SURFACE, width=2)
    c.text(cx, cy, "M", "body-bold", int(r * 1.15), SURFACE, anchor="mm")


def event_badge_width(c: Canvas, text):
    return max(22, c.textlen(text, "body-bold", 11) + 12)


def draw_event_badge(c: Canvas, x0, cy, text):
    w = event_badge_width(c, text)
    c.rect(x0, cy - 11, w, 22, fill=SURFACE, outline=EVENT_BORDER, width=2, radius=11)
    c.text(x0 + w / 2, cy, text, "body-bold", 11, TEXT_STRONG, anchor="mm")
    return w


def draw_closure_icon(c: Canvas, cx, cy, esito, r=12):
    c.circle(cx, cy, r, fill=CLOSURE[esito][0], outline=SURFACE, width=2)
    k = r * 0.45
    if esito == "ok":
        c.line(cx - k, cy, cx - k * 0.2, cy + k * 0.8, SURFACE, 3)
        c.line(cx - k * 0.2, cy + k * 0.8, cx + k, cy - k * 0.7, SURFACE, 3)
    else:
        c.line(cx - k, cy - k, cx + k, cy + k, SURFACE, 3)
        c.line(cx - k, cy + k, cx + k, cy - k, SURFACE, 3)


# ------------------------------------------------------------------ text helpers
def fit_lines(c: Canvas, text, key, size, maxw, maxl, what, failures):
    lines = c.wrap(text, key, size, maxw)
    if len(lines) > maxl or any(c.textlen(line, key, size) > maxw + 0.5 for line in lines):
        failures.append(f"{what}: «{text}» does not fit ({maxl} lines of {int(maxw)} px): shorten it")
        lines = lines[:maxl]
        lines[-1] = c.ellipsis(lines[-1] + "…", key, size, maxw)
    return lines


def flow_label_size(c: Canvas, f, stacked=False):
    """Lines of the trigger label, width of the event badge, total width and height of the label block.
    Stacked: the badge sits above a narrower label, for a label that has to fit in the gap between two nodes."""
    maxw = GAP_LABEL_MAX_W if stacked else LABEL_MAX_W
    lines = c.wrap(f["label"], "body-semi", LABEL_FONT, maxw) if f["label"] else []
    badge = event_badge_width(c, f["evento"]) if f["evento"] else 0
    text_w = (max(c.textlen(line, "body-semi", LABEL_FONT) for line in lines) + 14) if lines else 0
    text_h = len(lines) * LABEL_LINE_H + 8 if lines else 0
    if not lines and not badge:
        return [], 0, 0, 0
    if stacked:
        return lines, badge, max(badge, text_w), text_h + (26 if badge else 0)
    w = badge + (4 if badge and lines else 0) + text_w
    return lines, badge, w, max(text_h, 24 if badge else 0)


# ------------------------------------------------------------------ legend
def legend_items(spec, lanes, nodes, flows):
    """(key, default text, drawer) for every element the figure uses, in the order of the customer's legend."""
    items = []
    for k, (color, text) in FLOW_KINDS.items():
        if any(f["kind"] == k for f in flows):
            items.append((k, text, ("arrow", color, False)))
    if any(not f["integrato"] for f in flows):
        items.append(("integrato", "Passaggio tra applicativi integrati", ("arrow", UNINTEGRATED, False)))
        items.append(("non-integrato", "Passaggio non integrato, a carico dell'utente", ("arrow", UNINTEGRATED, True)))
    for tech, (color, text) in APP_TECH.items():
        if any(n["kind"] == "applicazione" and n["tecnologia"] == tech for n in nodes):
            items.append((tech, text, ("app", color, False)))
    if any(n.get("opzionale") for n in nodes):
        items.append(("opzionale", "Step di processo non obbligatorio", ("app", SURFACE, True)))
    for rec, (color, text) in MAIL_RECIPIENTS.items():
        if any(n["kind"] == "notifica" and n["destinatario"] == rec for n in nodes):
            items.append((f"mail-{rec}", text, ("mail", color)))
    if any(f["evento"] for f in flows):
        items.append(("evento", "ID evento", ("event",)))
    if any(f["label"] for f in flows):
        items.append(("trigger", "Trigger evento", ("trigger",)))
    if any(n.get("modale") for n in nodes):
        items.append(("modale", "Gestione con modale", ("modal",)))
    if any(n.get("evolutivo") for n in nodes):
        items.append(("evolutivo", "Intervento evolutivo pianificato o in corso", ("gear",)))
    if any(lane.get("primo_rilascio") for lane in lanes):
        items.append(("primo-rilascio", "Utente del primo rilascio FREE", ("ring",)))
    overrides = spec.get("legend") or {}
    return [(k, overrides.get(k, text), drawer) for k, text, drawer in items]


def draw_legend_sample(c: Canvas, x, cy, drawer):
    kind = drawer[0]
    if kind == "arrow":
        polyline(c, [(x, cy), (x + 30, cy)], drawer[1], 2, drawer[2])
        arrow_head(c, (x + 34, cy), (x, cy), drawer[1])
    elif kind == "app":
        c.rect(x + 2, cy - 8, 32, 16, fill=drawer[1], outline=None if drawer[2] else APP_BORDER, width=1)
        if drawer[2]:
            dashed_rect(c, x + 2, cy - 8, x + 34, cy + 8, APP_BORDER, 1)
    elif kind == "mail":
        draw_envelope(c, x + 5, cy - 10, 26, 20, drawer[1])
    elif kind == "event":
        draw_event_badge(c, x + 7, cy, "n")
    elif kind == "trigger":
        c.rect(x + 1, cy - 9, 34, 18, fill=SURFACE, outline=TEAL, width=1, radius=4)
        c.text(x + 18, cy, "text", "body-semi", 10, TEXT, anchor="mm")
    elif kind == "modal":
        draw_modal_badge(c, x + 18, cy, 10)
    elif kind == "gear":
        draw_gear(c, x + 18, cy, 8)
    elif kind == "ring":
        c.rect(x + 2, cy - 9, 32, 18, fill=USER_TAG, radius=9)
        c.rect(x, cy - 11, 36, 22, outline=FIRST_RELEASE_RING, width=2, radius=11)


# ------------------------------------------------------------------ render
def render(spec: dict, out_path: str) -> dict:
    m = build_model(spec)
    gap_tracks, gap_count, corridor_tracks, corridor_count = plan_routes(m)
    lanes, phases, nodes, flows = m["lanes"], m["phases"], m["nodes"], m["flows"]
    R, T = len(lanes), m["positions"]
    failures, warnings = [], list(m["warnings"])

    probe = Canvas(10, 10, SURFACE)  # measuring only
    for f in flows:
        f["stacked"] = f["mode"] == "straight" and f["t"]["pos"] == f["s"]["pos"] + 1
        f["lines"], f["badge_w"], f["lw"], f["lh"] = flow_label_size(probe, f, f["stacked"])

    # gap widths -------------------------------------------------------
    gap_w = []
    for g in range(T + 1):
        n = gap_count.get(g, 0)
        gap_w.append(max(GAP_MIN, 2 * GAP_PAD + max(n - 1, 0) * TRACK_SPACING))
    for f in flows:  # a straight flow between adjacent slots carries its label in the gap
        if f["mode"] == "straight" and f["t"]["pos"] == f["s"]["pos"] + 1 and f["lw"]:
            g = f["t"]["pos"]
            gap_w[g] = max(gap_w[g], f["lw"] + 20)

    # corridor heights ---------------------------------------------------
    corridor_h, track_h = [], {}
    for cidx in range(R + 1):
        n = corridor_count.get(cidx, 0)
        heights = [TRACK_H] * n
        for f in flows:
            if f["mode"] in ("corridor", "loop") and f["corridor"] == cidx and f["lh"]:
                k = corridor_tracks[cidx][f["i"]]
                heights[k] = max(heights[k], f["lh"])
        track_h[cidx] = heights
        corridor_h.append(max(CORRIDOR_MIN, 10 + sum(heights) + 4 * max(n - 1, 0)))

    # horizontal positions ----------------------------------------------
    x = MARGIN + LANE_HEADER_W
    gap_left, pos_left = [], []
    for g in range(T + 1):
        gap_left.append(x)
        x += gap_w[g]
        if g < T:
            pos_left.append(x)
            x += NODE_W
    lanes_right = x

    def track_x(g, k):
        n = gap_count.get(g, 1)
        return gap_left[g] + gap_w[g] / 2 + (k - (n - 1) / 2) * TRACK_SPACING

    # title and legend --------------------------------------------------
    items = legend_items(spec, lanes, list(nodes.values()), flows)
    columns = [items[i : i + LEGEND_ROWS] for i in range(0, len(items), LEGEND_ROWS)]
    col_w = [44 + max(probe.textlen(text, "body", LEGEND_FONT) for _, text, _ in col) + 18 for col in columns]
    legend_w = (sum(col_w) + 24) if items else 0
    legend_h = (34 + LEGEND_ROWS * LEGEND_ROW_H if len(items) > LEGEND_ROWS else 34 + len(items) * LEGEND_ROW_H) if items else 0
    title_w = max(probe.textlen(spec.get("title", "Vista di processo"), "heading", 26), probe.textlen(spec.get("subtitle", ""), "body-semi", 15))
    width = int(max(lanes_right + MARGIN, MARGIN * 2 + legend_w))
    legend_beside = MARGIN + title_w + 40 + legend_w <= width - MARGIN
    title_h = max(78, legend_h + 12) if legend_beside else 78 + legend_h + 12
    legend_x = width - MARGIN - legend_w if legend_beside else MARGIN
    legend_y = MARGIN if legend_beside else MARGIN + 78

    phase_y = MARGIN + title_h
    lanes_top = phase_y + PHASE_BAR_H + 10
    lane_top, y = [], lanes_top
    for r in range(R):
        lane_top.append(y)
        y += corridor_h[r] + BAND_H
    bottom_corridor_top = y
    lanes_bottom = y + corridor_h[R]

    def lane_cy(r):
        return lane_top[r] + corridor_h[r] + BAND_H / 2

    def corridor_track_y(cidx, k):
        top = lane_top[cidx] if cidx < R else bottom_corridor_top
        ys = top + 5
        for j in range(k):
            ys += track_h[cidx][j] + 4
        return ys + track_h[cidx][k] / 2

    # notes under the lanes -----------------------------------------------
    phase_x = []
    for p in range(len(phases)):
        first = m["offset"][p]
        last = first + m["slots"][p]
        x0 = gap_left[first] + (0 if first == 0 else gap_w[first] / 2)
        x1 = gap_left[last] + (gap_w[last] if last == T else gap_w[last] / 2)
        phase_x.append((x0, x1))
    note_lines, note_h = [], 0
    for p, ph in enumerate(phases):
        text = str(ph.get("note", "") or "").strip()
        ls = probe.wrap(text, "body", NOTE_FONT, phase_x[p][1] - phase_x[p][0] - 30) if text else []
        note_lines.append(ls)
        note_h = max(note_h, len(ls) * 18 + 20 if ls else 0)
    height = int(lanes_bottom + (16 + note_h if note_h else 0) + MARGIN)

    c = Canvas(width, height, SURFACE)

    # title and legend
    c.text(MARGIN, MARGIN + 4, spec.get("title", "Vista di processo"), "heading", 26, TEXT_STRONG)
    if spec.get("subtitle"):
        c.text(MARGIN, MARGIN + 44, spec["subtitle"], "body-semi", 15, TEAL)
    if items:
        c.rect(legend_x, legend_y, legend_w, legend_h, fill=SURFACE, outline=BORDER, width=1, radius=8)
        c.text(legend_x + 12, legend_y + 8, "LEGENDA", "body-bold", 12, TEXT_STRONG)
        cx0 = legend_x + 12
        for col, w in zip(columns, col_w):
            for j, (_, text, drawer) in enumerate(col):
                cy = legend_y + 34 + LEGEND_ROW_H * j + LEGEND_ROW_H / 2 - 2
                draw_legend_sample(c, cx0, cy, drawer)
                c.text(cx0 + 44, cy, text, "body", LEGEND_FONT, TEXT, anchor="lm")
            cx0 += w

    # phase bar ("blocchi funzionali")
    for p, ph in enumerate(phases):
        x0, x1 = phase_x[p]
        x0 += 2
        x1 -= 2
        tip = CHEVRON_TIP
        pts = [(x0, phase_y), (x1 - tip, phase_y), (x1, phase_y + PHASE_BAR_H / 2), (x1 - tip, phase_y + PHASE_BAR_H), (x0, phase_y + PHASE_BAR_H)]
        if p > 0:
            pts.append((x0 + tip, phase_y + PHASE_BAR_H / 2))
        polygon(c, pts, fill=TEAL_DARK if p % 2 else TEAL)
        label = str(ph.get("label", ph.get("id"))).upper()
        avail = x1 - x0 - 2 * tip - 8
        lines = fit_lines(c, label, "body-bold", 12, avail, 2, f"phase {ph.get('id')}", failures)
        cx = (x0 + x1) / 2
        for j, line in enumerate(lines):
            c.text(cx, phase_y + PHASE_BAR_H / 2 + (j - (len(lines) - 1) / 2) * 15, line, "body-bold", 12, SURFACE, anchor="mm")

    # lanes: one per user who owns the steps, with the user tag on the left
    for r, lane in enumerate(lanes):
        top = lane_top[r]
        bottom = lane_top[r + 1] if r + 1 < R else lanes_bottom
        c.rect(MARGIN + LANE_HEADER_W, top, lanes_right - MARGIN - LANE_HEADER_W, bottom - top, fill=PAGE_BG if r % 2 == 0 else SURFACE)
        c.rect(MARGIN, top, LANE_HEADER_W, bottom - top, fill="#FAFAFA")
        lines = fit_lines(c, str(lane.get("label", lane["id"])), "heading-semi", 14, LANE_HEADER_W - 44, 3, f"lane {lane['id']}", failures)
        tag_h = len(lines) * 18 + 16
        tag_w = LANE_HEADER_W - 24
        tx0, ty0 = MARGIN + 12, (top + bottom) / 2 - tag_h / 2
        c.rect(tx0, ty0, tag_w, tag_h, fill=USER_TAG if lane.get("kind", "utente") == "utente" else SYSTEMS_TAG, radius=12)
        if lane.get("primo_rilascio"):
            c.rect(tx0 - 4, ty0 - 4, tag_w + 8, tag_h + 8, outline=FIRST_RELEASE_RING, width=3, radius=15)
        for j, line in enumerate(lines):
            c.text(tx0 + tag_w / 2, ty0 + 8 + 9 + j * 18, line, "heading-semi", 14, TEXT_STRONG, anchor="mm")
        if r > 0:  # the customer separates the lanes with a dashed line
            x_ = MARGIN
            while x_ < lanes_right:
                c.line(x_, top, min(x_ + 8, lanes_right), top, MUTED_1, 1)
                x_ += 14
    c.line(MARGIN, lanes_top, lanes_right, lanes_top, BORDER, 1)
    c.line(MARGIN, lanes_bottom, lanes_right, lanes_bottom, BORDER, 1)
    c.line(MARGIN + LANE_HEADER_W, lanes_top, MARGIN + LANE_HEADER_W, lanes_bottom, MUTED_1, 1)
    for p in range(1, len(phases)):
        dashed_vline(c, phase_x[p][0], lanes_top, lanes_bottom, MUTED_1)

    # node geometry and ports ------------------------------------------------
    def node_box(n):
        """The shape the arrows attach to."""
        cx = pos_left[n["pos"]] + NODE_W / 2
        cy = lane_cy(n["r"])
        if n["kind"] == "notifica":
            cy -= 14  # leaves room for the label under the envelope
            return cx - MAIL_W / 2, cy - MAIL_H / 2, cx + MAIL_W / 2, cy + MAIL_H / 2
        h = {"decisione": DIAMOND_H, "avvio": PILL_H, "chiusura": PILL_H}.get(n["kind"], NODE_H)
        return cx - NODE_W / 2, cy - h / 2, cx + NODE_W / 2, cy + h / 2

    def other_level(f, end):
        if f["mode"] == "column":
            return f["s"]["pos"] + 0.5  # top/bottom ports are ordered left to right
        if f["mode"] == "loop" and end == "exit":
            return f["gap_entry"]
        if f["mode"] in ("corridor", "loop"):
            return 2 * f["corridor"]
        n = f["t"] if end == "exit" else f["s"]
        return 2 * n["r"] + 1

    sides = {}
    for f in flows:
        sides.setdefault((f["s"]["id"], f["exit"]), []).append((other_level(f, "exit"), f["i"], "exit"))
        sides.setdefault((f["t"]["id"], f.get("entry", "left")), []).append((other_level(f, "entry"), f["i"], "entry"))
    port = {}
    for (nid, side), port_items in sides.items():
        n = nodes[nid]
        x0, y0, x1, y1 = node_box(n)
        w, h = x1 - x0, y1 - y0
        cx, cy = (x0 + x1) / 2, (y0 + y1) / 2
        port_items.sort()
        k = len(port_items)
        if side in ("top", "bottom"):
            limit = (w - h) / 2 if n["kind"] in ("avvio", "chiusura") else w / 2 - 14
            for j, (_, fi, end) in enumerate(port_items):
                dx = max(-limit, min(limit, (j - (k - 1) / 2) * 14))
                inset = (h / 2) * (abs(dx) / (w / 2)) if n["kind"] == "decisione" else 0
                port[(fi, end)] = (cx + dx, y0 + inset if side == "top" else y1 - inset)
            continue
        for j, (_, fi, end) in enumerate(port_items):
            dy = max(-(h / 2 - 8), min(h / 2 - 8, (j - (k - 1) / 2) * PORT_SPACING))
            inset = (w / 2) * (abs(dy) / (h / 2)) if n["kind"] == "decisione" else 0
            px = x0 + inset if side == "left" else x1 - inset
            port[(fi, end)] = (px, cy + dy)

    # paths ------------------------------------------------------------------
    for f in flows:
        sx, sy = port[(f["i"], "exit")]
        tx, ty = port[(f["i"], "entry")]
        if f["mode"] == "straight":
            mid = (sx + tx) / 2 if f["t"]["pos"] > f["s"]["pos"] + 1 else gap_left[f["t"]["pos"]] + gap_w[f["t"]["pos"]] / 2
            pts = [(sx, sy), (tx, ty)] if abs(sy - ty) < 0.5 else [(sx, sy), (mid, sy), (mid, ty), (tx, ty)]
        elif f["mode"] == "vertical":
            gx = track_x(f["gap_exit"], gap_tracks[f["gap_exit"]][(f["i"], "v")])
            pts = [(sx, sy), (gx, sy), (gx, ty), (tx, ty)]
        elif f["mode"] == "column":
            my = (sy + ty) / 2
            pts = [(sx, sy), (tx, ty)] if abs(sx - tx) < 0.5 else [(sx, sy), (sx, my), (tx, my), (tx, ty)]
        elif f["mode"] == "loop":
            gxi = track_x(f["gap_entry"], gap_tracks[f["gap_entry"]][(f["i"], "v2")])
            cy = corridor_track_y(f["corridor"], corridor_tracks[f["corridor"]][f["i"]])
            pts = [(sx, sy), (sx, cy), (gxi, cy), (gxi, ty), (tx, ty)]
        else:
            gxe = track_x(f["gap_exit"], gap_tracks[f["gap_exit"]][(f["i"], "v1")])
            gxi = track_x(f["gap_entry"], gap_tracks[f["gap_entry"]][(f["i"], "v2")])
            cy = corridor_track_y(f["corridor"], corridor_tracks[f["corridor"]][f["i"]])
            pts = [(sx, sy), (gxe, sy), (gxe, cy), (gxi, cy), (gxi, ty), (tx, ty)]
        f["pts"] = pts

    for f in flows:
        color = FLOW_KINDS[f["kind"]][0]
        polyline(c, f["pts"], color, 2, dashed=not f["integrato"])
        arrow_head(c, f["pts"][-1], f["pts"][-2], color)

    # nodes ----------------------------------------------------------------------
    obstacles = []
    for n in nodes.values():
        x0, y0, x1, y1 = node_box(n)
        cx, cy = (x0 + x1) / 2, (y0 + y1) / 2
        what = f"node {n['id']}"
        if n["kind"] == "applicazione":
            fill = APP_TECH[n["tecnologia"]][0]
            c.rect(x0 + 2, y0 + 3, NODE_W, y1 - y0, fill="#E4ECEE")
            c.rect(x0, y0, NODE_W, y1 - y0, fill=fill, outline=None if n.get("opzionale") else APP_BORDER, width=2)
            if n.get("opzionale"):
                dashed_rect(c, x0, y0, x1, y1, APP_BORDER, 2)
            name = fit_lines(c, str(n["label"]), "body-bold", NODE_FONT, NODE_W - 16, 1, what, failures)
            detail = fit_lines(c, str(n["attivita"]), "body", 13, NODE_W - 16, 2, what, failures) if n.get("attivita") else []
            block = 20 + len(detail) * 16
            yy = cy - block / 2
            c.text(cx, yy + 10, name[0], "body-bold", NODE_FONT, TEXT_STRONG, anchor="mm")
            for j, line in enumerate(detail):
                c.text(cx, yy + 20 + 8 + j * 16, line, "body", 13, TEXT_STRONG, anchor="mm")
        elif n["kind"] == "decisione":
            polygon(c, [(cx, y0), (x1, cy), (cx, y1), (x0, cy)], fill="#EEF1F2", outline=TEXT_SECONDARY, width=2)
            lines = fit_lines(c, str(n["label"]), "body-semi", 13, NODE_W * 0.62, 3, what, failures)
            for j, line in enumerate(lines):
                c.text(cx, cy + (j - (len(lines) - 1) / 2) * 16, line, "body-semi", 13, TEXT_STRONG, anchor="mm")
        elif n["kind"] == "notifica":
            draw_envelope(c, x0, y0, MAIL_W, MAIL_H, MAIL_RECIPIENTS[n["destinatario"]][0])
            lines = fit_lines(c, str(n["label"]), "body-semi", 13, NODE_W - 6, 2, what, failures)
            for j, line in enumerate(lines):
                c.text(cx, y1 + 12 + j * 16, line, "body-semi", 13, TEXT_STRONG, anchor="mm")
            obstacles.append((cx - NODE_W / 2, y1, cx + NODE_W / 2, y1 + 6 + 16 * len(lines)))
        elif n["kind"] == "avvio":
            c.rect(x0, y0, NODE_W, y1 - y0, fill=TEAL, radius=(y1 - y0) / 2)
            lines = fit_lines(c, str(n["label"]), "body-bold", NODE_FONT - 1, NODE_W - 30, 2, what, failures)
            for j, line in enumerate(lines):
                c.text(cx, cy + (j - (len(lines) - 1) / 2) * 18, line, "body-bold", NODE_FONT - 1, SURFACE, anchor="mm")
        else:  # chiusura
            c.rect(x0, y0, NODE_W, y1 - y0, fill=TEAL_DARKER, radius=(y1 - y0) / 2)
            draw_closure_icon(c, x0 + 24, cy, n["esito"])
            lines = fit_lines(c, str(n["label"]).upper(), "body-bold", 12, NODE_W - 50, 3, what, failures)
            for j, line in enumerate(lines):
                c.text(x0 + 42 + (NODE_W - 50) / 2, cy + (j - (len(lines) - 1) / 2) * 15, line, "body-bold", 12, SURFACE, anchor="mm")
        if n.get("modale"):
            draw_modal_badge(c, x0 + 2, y0 + 2)
        if n.get("evolutivo"):
            draw_gear(c, x1 - 2, y0 + 2)
        obstacles.append((x0, y0 - (12 if n.get("modale") or n.get("evolutivo") else 0), x1, y1))

    # flow labels: trigger and event ID, along their own path where they hide the fewest other lines
    segments = [(f["i"], a, b) for f in flows for a, b in zip(f["pts"], f["pts"][1:])]

    def crossings(box, own):
        x0, y0, x1, y1 = box
        hits = 0
        for i, (ax_, ay_), (bx_, by_) in segments:
            if i == own:
                continue
            if abs(ay_ - by_) < 0.5 and y0 <= ay_ <= y1 and min(ax_, bx_) < x1 and max(ax_, bx_) > x0:
                hits += 1
            elif abs(ax_ - bx_) < 0.5 and x0 <= ax_ <= x1 and min(ay_, by_) < y1 and max(ay_, by_) > y0:
                hits += 1
        return hits

    label_boxes = []
    for f in sorted(flows, key=lambda f: f["i"]):
        if not f["lw"]:
            continue
        w, h = f["lw"], f["lh"]
        pts = f["pts"]
        longest = max(zip(pts, pts[1:]), key=lambda s: abs(s[0][0] - s[1][0]) + abs(s[0][1] - s[1][1]))
        home = ((longest[0][0] + longest[1][0]) / 2, (longest[0][1] + longest[1][1]) / 2)
        best = None
        for (ax_, ay_), (bx_, by_) in zip(pts, pts[1:]):
            length = abs(bx_ - ax_) + abs(by_ - ay_)
            steps = max(1, int(length // 12))
            # on a vertical segment the label may also sit beside the line, touching it
            shifts = (0, -(w / 2 + 3), w / 2 + 3) if abs(ax_ - bx_) < 0.5 and not f["stacked"] else (0,)
            for k in range(steps + 1):
                for shift in shifts:
                    cx = ax_ + (bx_ - ax_) * k / steps + shift
                    cy = ay_ + (by_ - ay_) * k / steps
                    box = (cx - w / 2, cy - h / 2, cx + w / 2, cy + h / 2)
                    hard = sum(overlaps(box, b) for b in label_boxes) + sum(overlaps(box, b, 0) for b in obstacles)
                    hard += box[0] < MARGIN + LANE_HEADER_W + 2 or box[2] > lanes_right - 2
                    score = 1000 * hard + 40 * crossings(box, f["i"]) + (abs(cx - home[0]) + abs(cy - home[1])) / 50 + (8 if shift else 0)
                    if best is None or score < best[0]:
                        best = (score, box, hard)
        _, chosen, hard = best
        if hard:
            warnings.append(f"flow {f['i'] + 1} ({f['s']['id']} → {f['t']['id']}): label overlaps another label or a node; shorten it or move a node")
        label_boxes.append(chosen)
        x0, y0, x1, y1 = chosen
        color = FLOW_KINDS[f["kind"]][0]
        lh = len(f["lines"]) * LABEL_LINE_H + 8
        if f["stacked"]:
            mid = (x0 + x1) / 2
            if f["evento"]:
                draw_event_badge(c, mid - f["badge_w"] / 2, y0 + 11, f["evento"])
            if f["lines"]:
                tw = max(c.textlen(line, "body-semi", LABEL_FONT) for line in f["lines"]) + 14
                tx, ty = mid - tw / 2, y1 - lh
                c.rect(tx, ty, tw, lh, fill=SURFACE, outline=color, width=1, radius=4)
                for j, line in enumerate(f["lines"]):
                    c.text(mid, ty + 4 + LABEL_LINE_H / 2 + j * LABEL_LINE_H, line, "body-semi", LABEL_FONT, TEXT, anchor="mm")
            continue
        cy = (y0 + y1) / 2
        lx = x0
        if f["evento"]:
            lx += draw_event_badge(c, x0, cy, f["evento"]) + 4
        if f["lines"]:
            c.rect(lx, cy - lh / 2, x1 - lx, lh, fill=SURFACE, outline=color, width=1, radius=4)
            for j, line in enumerate(f["lines"]):
                c.text((lx + x1) / 2, cy - lh / 2 + 4 + LABEL_LINE_H / 2 + j * LABEL_LINE_H, line, "body-semi", LABEL_FONT, TEXT, anchor="mm")

    # phase notes ----------------------------------------------------------------------
    if note_h:
        ny = lanes_bottom + 16
        for p in range(len(phases)):
            if not note_lines[p]:
                continue
            x0, x1 = phase_x[p]
            c.rect(x0 + 4, ny, x1 - x0 - 8, note_h, fill=TINT_300, outline=MUTED_1, width=1, radius=8)
            for j, line in enumerate(note_lines[p]):
                c.text(x0 + 18, ny + 10 + j * 18, line, "body", NOTE_FONT, TEXT)

    os.makedirs(os.path.dirname(out_path) or ".", exist_ok=True)
    c.save(out_path)
    smallest = min(LABEL_FONT, NOTE_FONT, LEGEND_FONT, 13)
    smallest_pt = smallest * (LANDSCAPE_IMAGE_CM / width) / 0.03528
    if height / width > LANDSCAPE_IMAGE_MAX_HEIGHT_CM / LANDSCAPE_IMAGE_CM:
        smallest_pt = min(smallest_pt, smallest * (LANDSCAPE_IMAGE_MAX_HEIGHT_CM / height) / 0.03528)
    if smallest_pt < MIN_READABLE_PT:
        warnings.append(
            f"smallest text ≈ {smallest_pt:.1f} pt on a landscape A4 page: merge nodes, shorten the phases "
            "or split the process into two figures"
        )
    return {
        "path": out_path,
        "size": os.path.getsize(out_path),
        "width": width,
        "height": height,
        "smallest_pt": smallest_pt,
        "counts": (len(phases), len(lanes), len(nodes), len(flows)),
        "failures": failures,
        "warnings": warnings,
        "fonts": c.font_report,
    }


# ------------------------------------------------------------------ example and CLI
EXAMPLE = {
    "id": "processo",
    "title": "Estinzione anticipata finanziamento",
    "subtitle": "Vista di processo: fasi, utenti e applicativi (dati esemplificativi)",
    "phases": [
        {"id": "richiesta", "label": "Richiesta", "note": "L'operatore avvia l'estinzione dalla scheda del rapporto; FREE apre la pratica."},
        {"id": "conteggio", "label": "Conteggio estintivo", "note": "Il conteggio viene calcolato e confermato dall'operatore."},
        {"id": "autorizzazione", "label": "Autorizzazione", "note": "Sopra soglia serve il benestare del back office, avvisato via mail."},
        {"id": "chiusura", "label": "Chiusura", "note": "Il rapporto viene estinto e la pratica si chiude in FREE."},
    ],
    "lanes": [
        {"id": "rete", "label": "Utente Rete", "kind": "utente", "primo_rilascio": True},
        {"id": "backoffice", "label": "Back office crediti", "kind": "utente"},
        {"id": "sistemi", "label": "Sistemi", "kind": "sistemi"},
    ],
    "nodes": [
        {"id": "avvio", "lane": "rete", "phase": "richiesta", "kind": "avvio", "label": "Avvio processo"},
        {"id": "free", "lane": "rete", "phase": "richiesta", "label": "FREE", "tecnologia": "web", "attivita": "Apre la pratica"},
        {"id": "mdweb", "lane": "rete", "phase": "conteggio", "label": "MDWEB", "tecnologia": "web", "attivita": "Verifica il conteggio", "modale": True},
        {"id": "soglia", "lane": "rete", "phase": "conteggio", "kind": "decisione", "label": "Sopra soglia?"},
        {"id": "calcolo", "lane": "sistemi", "phase": "conteggio", "label": "MD00", "tecnologia": "sistema", "attivita": "Calcola il debito"},
        {"id": "mail", "lane": "backoffice", "phase": "autorizzazione", "kind": "notifica", "destinatario": "pool", "label": "Richiesta benestare"},
        {"id": "benestare", "lane": "backoffice", "phase": "autorizzazione", "label": "GEFI", "tecnologia": "3270", "attivita": "Rilascia benestare", "evolutivo": True},
        {"id": "estinzione", "lane": "sistemi", "phase": "chiusura", "label": "MD00", "tecnologia": "sistema", "attivita": "Estingue il rapporto"},
        {"id": "ok", "lane": "rete", "phase": "chiusura", "kind": "chiusura", "esito": "ok"},
        {"id": "ko", "lane": "backoffice", "phase": "chiusura", "kind": "chiusura", "esito": "ko"},
    ],
    "flows": [
        {"from": "avvio", "to": "free", "evento": "1"},
        {"from": "free", "to": "mdweb", "label": "Avvio conteggio", "evento": "2"},
        {"from": "mdweb", "to": "calcolo", "label": "Richiesta conteggio"},
        {"from": "calcolo", "to": "soglia", "label": "Conteggio pronto", "evento": "3"},
        {"from": "soglia", "to": "mail", "label": "Sopra soglia"},
        {"from": "mail", "to": "benestare", "integrato": False},
        {"from": "soglia", "to": "estinzione", "label": "Sotto soglia", "evento": "4"},
        {"from": "benestare", "to": "estinzione", "label": "Benestare OK", "evento": "5"},
        {"from": "benestare", "to": "ko", "label": "Benestare KO", "kind": "annullo", "evento": "6"},
        {"from": "benestare", "to": "mdweb", "label": "Conteggio da rifare", "kind": "ricircolo"},
        {"from": "estinzione", "to": "ok", "label": "Rapporto estinto", "evento": "7"},
    ],
}


def output_name(spec: dict) -> str:
    slug = str(spec.get("id", "processo")).strip().replace(" ", "-").replace(".", "-") or "processo"
    return f"swimlane-{slug}.png"


def run(files, out_dir) -> int:
    print("== render_process report ==")
    total_fail = 0
    for f in files:
        try:
            with open(f, encoding="utf-8") as fh:
                spec = json.load(fh)
            r = render(spec, os.path.join(out_dir, output_name(spec)))
        except (SpecError, json.JSONDecodeError, OSError) as exc:
            total_fail += 1
            print(f"  {os.path.basename(f)}: ERROR {type(exc).__name__}: {exc}")
            continue
        ph, ln, nd, fl = r["counts"]
        print(
            f"  {os.path.basename(f)} -> {os.path.basename(r['path'])}  {r['size'] / 1024:.0f} KB  {r['width']}x{r['height']}"
            f"  phases={ph} lanes={ln} nodes={nd} flows={fl}  smallest text≈{r['smallest_pt']:.1f} pt"
        )
        for msg in r["failures"]:
            print(f"    FAIL {msg}")
        for msg in r["warnings"]:
            print(f"    WARN {msg}")
        total_fail += len(r["failures"])
    print(f"spec files: {len(files)} · failures: {total_fail}")
    print("RESULT: " + ("OK" if not total_fail else "CHECK FAILED"))
    return 0 if not total_fail else 2


def main(argv=None):
    try:
        sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    except (AttributeError, ValueError):
        pass
    ap = argparse.ArgumentParser(description="Process swimlane spec (JSON) -> PNG")
    ap.add_argument("specs", nargs="*", help="JSON spec files (globs allowed)")
    ap.add_argument("--out", default="screens")
    ap.add_argument("--check", action="store_true", help="verify fonts and render the built-in example")
    ap.add_argument("--example", action="store_true", help="print the built-in example spec")
    args = ap.parse_args(argv)
    if args.example:
        print(json.dumps(EXAMPLE, ensure_ascii=False, indent=2))
        return 0
    if args.check:
        os.makedirs(args.out, exist_ok=True)
        path = os.path.join(args.out, "esempio-processo.json")
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(EXAMPLE, fh, ensure_ascii=False, indent=2)
        return run([path], args.out)
    files = []
    for pattern in args.specs:
        files.extend(sorted(glob.glob(pattern)) or [pattern])
    if not files:
        ap.error("no spec files given (or use --check / --example)")
    return run(files, args.out)


if __name__ == "__main__":
    sys.exit(main())
