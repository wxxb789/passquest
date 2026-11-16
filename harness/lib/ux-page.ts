/**
 * The in-page audit library of the desktop-ux gate: page JavaScript that
 * defines `UX` in a smoke-hook script. It measures the rendered window only
 * through the DOM and computed style; it uses no app internal beyond the public
 * DOM hooks. Validated against screenshot pixels during goal 0012 discovery
 * (548 samples, no case a pixel failed and the computation passed).
 *
 * Findings are `{ check, el, ...detail }`, where `el` is a short element path.
 * Checks:
 * - layout: page-h-overflow, text-clipped-x, text-clipped-y, text-cut-by-ancestor,
 *   outside-viewport-x, interactive-covered, interactive-not-hit, interactive-offscreen,
 *   scroll-without-content (a scroll container that scrolls although its content and end padding
 *   fit: decoration such as a pseudo-element makes a scrollbar that reveals nothing),
 *   unreachable (text or a control out of view that no scrolling a learner can do brings into view:
 *   once every ancestor that scrolls (overflow auto or scroll) has scrolled to it, an ancestor that
 *   does not (overflow hidden or clip), or the viewport of a page that does not scroll, still cuts it)
 * - primary action: primary-missing, primary-ambiguous, primary-hidden, primary-covered
 * - must-see: must-see-hidden (an element the app marks [data-must-see], such as the consent and
 *   budget rows of a call strip, is not wholly inside the viewport and every clipping ancestor:
 *   the learner would have to scroll a container to find it)
 * - contrast (WCAG AA, 4.5 or 3 for large text) over the composited layers:
 *   contrast-fail when even the best background candidate fails,
 *   contrast-position when only some candidates fail (the text's position decides);
 *   text below the fold is scrolled into view to be measured, and form controls
 *   are measured on the text they draw (value, placeholder, chosen option)
 * - type: font-under-12px (text and generated ::before/::after text)
 * - keyboard: not-tabbable, focus-refused, no-focus-indicator, modal-not-trapping
 * - language: zh-ascii-words, en-cjk-text, raw-code
 * - motion (see motionReport/transitionReport/roomTransitions): motion-under-reduce, and
 *   transition-under-reduce (a CSS transition exists at all under reduced motion: a style change must
 *   apply at once, not through a short transition that a window WebKit does not show never advances)
 *
 * A hidden window never has system focus, so :focus, :focus-visible and
 * :focus-within never match; the focus check mirrors those rules onto the
 * .qa-fv/.qa-fw classes and sets them on the element it focused.
 */
export const UX_LIBRARY = String.raw`
const UX = (() => {
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = 1;
  const cx = canvas.getContext("2d", { willReadFrequently: true });
  const cache = new Map();
  // Any CSS colour (rgb, oklch, color(), ...) -> [r, g, b, a]: sRGB 0..255, alpha 0..1. The canvas converts.
  function rgba(str) {
    if (cache.has(str)) return cache.get(str);
    let out;
    const m = /^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:\s*[,/]\s*([\d.]+%?))?\s*\)$/.exec(str);
    if (m) {
      const a = m[4] === undefined ? 1 : m[4].endsWith("%") ? parseFloat(m[4]) / 100 : parseFloat(m[4]);
      out = [+m[1], +m[2], +m[3], a];
    } else {
      cx.clearRect(0, 0, 1, 1);
      cx.fillStyle = "rgba(0,0,0,0)";
      cx.fillStyle = str;
      cx.fillRect(0, 0, 1, 1);
      const d = cx.getImageData(0, 0, 1, 1).data;
      // The canvas stores 8-bit alpha; the string's own alpha is exact.
      let a = d[3] / 255;
      const am = /\/\s*([\d.]+%?)\s*\)$/.exec(str);
      if (am) a = am[1].endsWith("%") ? parseFloat(am[1]) / 100 : parseFloat(am[1]);
      if (str === "transparent") a = 0;
      out = [d[0], d[1], d[2], a];
    }
    cache.set(str, out);
    return out;
  }
  const over = (top, bottom) => {
    const a = top[3] + bottom[3] * (1 - top[3]);
    if (a === 0) return [0, 0, 0, 0];
    return [0, 1, 2].map((i) => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a).concat(a);
  };
  const lum = (c) => {
    const f = (v) => {
      v /= 255;
      return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
    };
    return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2]);
  };
  const ratio = (a, b) => {
    const x = Math.max(lum(a), lum(b));
    const y = Math.min(lum(a), lum(b));
    return (x + 0.05) / (y + 0.05);
  };
  const COLOR_TOKEN = /(rgba?\([^()]*\)|oklch\([^()]*\)|oklab\([^()]*\)|lab\([^()]*\)|lch\([^()]*\)|color\([^()]*\)|#[0-9a-fA-F]{3,8}\b|transparent)/g;

  const desc = (el) => {
    if (!el || el.nodeType !== 1) return String(el);
    let s = el.tagName.toLowerCase();
    if (el.id) s += "#" + el.id;
    // An id names the element; its classes (some transient, such as enter-layer) would only split groups.
    const cls = el.id ? [] : Array.from(el.classList).filter((c) => !/^qa-/.test(c)).slice(0, 2);
    if (cls.length) s += "." + cls.join(".");
    for (const a of ["data-act", "data-v", "data-testid"]) if (el.hasAttribute(a)) s += "[" + a + "=" + el.getAttribute(a) + "]";
    return s;
  };
  const path = (el) => {
    const parts = [];
    for (let e = el, i = 0; e && e !== document.body && i < 4; e = e.parentElement, i++) parts.unshift(desc(e));
    return parts.join(" > ");
  };
  const ownText = (el) =>
    Array.from(el.childNodes)
      .filter((n) => n.nodeType === 3)
      .map((n) => n.textContent)
      .join("")
      .replace(/\s+/g, " ")
      .trim();
  const visible = (el) => {
    if (!el.isConnected || el.closest("[inert]")) return false;
    if (!el.checkVisibility({ opacityProperty: true, visibilityProperty: true, contentVisibilityAuto: true })) return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  const effOpacity = (el) => {
    let o = 1;
    for (let e = el; e; e = e.parentElement) o *= parseFloat(getComputedStyle(e).opacity);
    return o;
  };
  const viewport = () => ({ vw: document.documentElement.clientWidth, vh: document.documentElement.clientHeight });
  // The part of el left visible by every clipping ancestor (overflow other than visible).
  const clipRect = (el) => {
    const r = el.getBoundingClientRect();
    let l = r.left, t = r.top, ri = r.right, b = r.bottom;
    let clippedBy = null;
    for (let e = el.parentElement; e && e !== document.documentElement; e = e.parentElement) {
      const cs = getComputedStyle(e);
      if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
      const p = e.getBoundingClientRect();
      if (cs.overflowX !== "visible") {
        if (l < p.left - 1 || ri > p.right + 1) clippedBy = clippedBy || { el: e, axis: "x", scroll: /auto|scroll/.test(cs.overflowX) };
        l = Math.max(l, p.left);
        ri = Math.min(ri, p.right);
      }
      if (cs.overflowY !== "visible") {
        if (t < p.top - 1 || b > p.bottom + 1) clippedBy = clippedBy || { el: e, axis: "y", scroll: /auto|scroll/.test(cs.overflowY) };
        t = Math.max(t, p.top);
        b = Math.min(b, p.bottom);
      }
    }
    return { l: l, t: t, r: ri, b: b, w: ri - l, h: b - t, clippedBy: clippedBy };
  };
  const INTERACTIVE = "[data-act],button,a[href],input:not([type=hidden]),select,textarea,[tabindex],[role=button],summary";
  const CONTROLS = "[data-act],button,a[href],input:not([type=hidden]),select,textarea,[role=button]";
  const focusable = (el) => el.tabIndex >= 0 && !el.disabled && !el.closest("[inert]");
  const SKIP_TAGS = new Set(["SCRIPT", "STYLE", "TEMPLATE", "svg", "path", "g", "use", "circle", "line", "rect", "polyline", "polygon", "defs", "SVG", "PATH"]);

  // The open modal layer is the audited root; otherwise the whole page.
  function root() {
    const layer = document.querySelector("#layer");
    const modal = layer && layer.querySelector('[aria-modal="true"]');
    return modal && visible(modal) ? { el: layer, modal: true } : { el: document.body, modal: false };
  }

  // ---------- contrast ----------
  // pointer-events:none positioned layers (the .atmos backdrop) are invisible to elementsFromPoint.
  let peLayers = [];
  function collectPeLayers() {
    peLayers = Array.from(document.querySelectorAll("body *")).filter((e) => {
      const cs = getComputedStyle(e);
      return cs.pointerEvents === "none" && /fixed|absolute/.test(cs.position) && (rgba(cs.backgroundColor)[3] > 0 || cs.backgroundImage !== "none") && visible(e);
    });
  }
  const saturate = (c, s) => {
    const m = [0.213 + 0.787 * s, 0.715 - 0.715 * s, 0.072 - 0.072 * s, 0.213 - 0.213 * s, 0.715 + 0.285 * s, 0.072 - 0.072 * s, 0.213 - 0.213 * s, 0.715 - 0.715 * s, 0.072 + 0.928 * s];
    const cl = (v) => Math.max(0, Math.min(255, v));
    return [cl(m[0] * c[0] + m[1] * c[1] + m[2] * c[2]), cl(m[3] * c[0] + m[4] * c[1] + m[5] * c[2]), cl(m[6] * c[0] + m[7] * c[1] + m[8] * c[2]), c[3]];
  };
  // Background candidates under the centre of rect: every gradient stop, translucent layer,
  // backdrop saturate() and optional pseudo-element layer, kept to the luminance extremes.
  function backgroundOf(el, rect) {
    const flags = new Set();
    const px = (rect.left + rect.right) / 2;
    const py = (rect.top + rect.bottom) / 2;
    let stack = [];
    const hits = document.elementsFromPoint(px, py);
    const i = hits.indexOf(el);
    if (i >= 0) {
      stack = hits.slice(i);
      const anc = [];
      for (let e = el; e; e = e.parentElement) anc.push(e);
      if (anc.some((a) => !stack.includes(a))) stack = Array.from(new Set(anc.concat(stack)));
      if (stack.some((s) => !s.contains(el))) flags.add("sibling-layer");
    } else {
      flags.add("nohit");
      for (let e = el; e; e = e.parentElement) stack.push(e);
    }
    const under = peLayers.filter((p) => {
      if (stack.includes(p) || p.contains(el) || el.contains(p)) return false;
      const r = p.getBoundingClientRect();
      return px >= r.left && px <= r.right && py >= r.top && py <= r.bottom;
    });
    if (under.length) {
      flags.add("pe-none-layer");
      const bi = stack.indexOf(document.body);
      stack.splice(bi < 0 ? stack.length : bi, 0, ...under);
    }
    const layers = [];
    let opaque = false;
    for (const e of stack) {
      const cs = getComputedStyle(e);
      let sat = null;
      if (cs.backdropFilter && cs.backdropFilter !== "none") {
        flags.add("backdrop-filter");
        const m = /saturate\(([\d.]+)\)/.exec(cs.backdropFilter);
        if (m) sat = parseFloat(m[1]);
        if (/brightness|contrast|invert|sepia|hue-rotate|grayscale|opacity/.test(cs.backdropFilter)) flags.add("backdrop-colour-op");
      }
      if (cs.mixBlendMode !== "normal") flags.add("blend");
      if (cs.filter !== "none" && e !== el) flags.add("filter");
      if (cs.maskImage && cs.maskImage !== "none") flags.add("mask");
      for (const pe of ["::after", "::before"]) {
        const ps = getComputedStyle(e, pe);
        if (ps.content === "none" || ps.content === "normal" || ps.position === "static") continue;
        if (/url\(/.test(ps.backgroundImage)) {
          flags.add(ps.mixBlendMode !== "normal" && parseFloat(ps.opacity) < 0.1 ? "noise-overlay" : "image");
          continue;
        }
        const cols = [];
        if (rgba(ps.backgroundColor)[3] > 0) cols.push(rgba(ps.backgroundColor));
        if (ps.backgroundImage !== "none") cols.push(...(ps.backgroundImage.match(COLOR_TOKEN) || []).map(rgba));
        if (cols.length) {
          flags.add("pseudo-layer");
          const po = parseFloat(ps.opacity);
          layers.push({ colors: cols.map((c) => [c[0], c[1], c[2], c[3] * po]), optional: true });
        }
      }
      const bimg = cs.backgroundImage;
      if (bimg && bimg !== "none") {
        if (/url\(/.test(bimg)) flags.add("image");
        const stops = (bimg.match(COLOR_TOKEN) || []).map(rgba);
        if (stops.length) {
          flags.add("gradient");
          layers.push({ colors: stops, sat: sat });
          sat = null;
        }
      }
      const bg = rgba(cs.backgroundColor);
      if (bg[3] > 0) {
        if (bg[3] < 1) flags.add("translucent");
        layers.push({ colors: [bg], sat: sat });
        if (bg[3] >= 1) {
          opaque = true;
          break;
        }
      } else if (sat) layers.push({ colors: [[0, 0, 0, 0]], sat: sat });
    }
    let candidates = [[255, 255, 255, 1]];
    if (!opaque) flags.add("canvas-white");
    for (const layer of layers.reverse()) {
      let base = candidates;
      if (layer.sat) base = base.concat(base.map((c) => saturate(c, layer.sat)));
      const next = layer.optional ? base.slice() : [];
      for (const c of base) for (const top of layer.colors) next.push(over(top, c));
      next.sort((p, q) => lum(p) - lum(q));
      candidates = next.length > 24 ? next.slice(0, 12).concat(next.slice(-12)) : next;
    }
    return { candidates: candidates, flags: flags };
  }
  // The worst and the best ratio of a colour over the candidates, and the AA threshold.
  function contrast(el, color, rect, fontSize, weight) {
    const fg0 = rgba(color);
    const fg = [fg0[0], fg0[1], fg0[2], fg0[3] * effOpacity(el)];
    const bg = backgroundOf(el, rect);
    const ratios = bg.candidates.map((c) => ratio(over(fg, c), c));
    const large = fontSize >= 24 || (fontSize >= 18.66 && weight >= 700);
    return { min: Math.min(...ratios), best: Math.max(...ratios), need: large ? 3 : 4.5, flags: Array.from(bg.flags).join(",") };
  }
  function contrastFinding(add, el, c, extra) {
    if (c.best < c.need) add("contrast-fail", el, Object.assign({ ratio: +c.min.toFixed(2), best: +c.best.toFixed(2), need: c.need, flags: c.flags }, extra));
    else if (c.min < c.need) add("contrast-position", el, Object.assign({ ratio: +c.min.toFixed(2), best: +c.best.toFixed(2), need: c.need, flags: c.flags }, extra));
  }
  // Some of the visible part of c lies inside the viewport, where the composited background can be sampled.
  const inView = (c, vh) => c.w > 0 && c.h > 0 && c.t < vh && c.b > 0;
  // An element's own text nodes, measured over the box of its first non-blank one.
  function textContrast(add, el, text) {
    const cs = getComputedStyle(el);
    const fs = parseFloat(cs.fontSize);
    const range = document.createRange();
    const node = Array.from(el.childNodes).find((n) => n.nodeType === 3 && n.textContent.trim());
    range.selectNodeContents(node);
    const tr = range.getBoundingClientRect();
    contrastFinding(add, el, contrast(el, cs.color, tr.width ? tr : el.getBoundingClientRect(), fs, parseInt(cs.fontWeight, 10)), { px: fs, text: text.slice(0, 40) });
  }
  // Text a form control draws without a text node: an input's value or placeholder, a select's chosen option.
  const NO_TEXT_INPUTS = /^(checkbox|radio|range|color|file|hidden|image)$/;
  function controlText(el, cs) {
    let text = "";
    let color = cs.color;
    if (el.tagName === "SELECT") text = el.selectedOptions[0] ? el.selectedOptions[0].text : "";
    else if (el.tagName === "TEXTAREA" || (el.tagName === "INPUT" && !NO_TEXT_INPUTS.test(el.type))) {
      if (el.tagName === "TEXTAREA" && ownText(el)) return null; // its text node is measured as text
      text = el.type === "password" ? "" : el.value;
      if (!text && el.placeholder) {
        text = el.placeholder;
        color = getComputedStyle(el, "::placeholder").color;
      }
    }
    text = text.replace(/\s+/g, " ").trim();
    return text ? { text: text, color: color } : null;
  }
  function controlContrast(add, el, control) {
    const cs = getComputedStyle(el);
    const fs = parseFloat(cs.fontSize);
    contrastFinding(add, el, contrast(el, control.color, el.getBoundingClientRect(), fs, parseInt(cs.fontWeight, 10)), { px: fs, text: control.text.slice(0, 40), control: true });
  }
  // Scroll containers and their offsets, to put back after measuring text below the fold.
  function scrollPositions() {
    const out = [[document.scrollingElement || document.documentElement, 0, 0]];
    for (const e of document.querySelectorAll("body *")) if (e.scrollTop || e.scrollLeft || e.scrollHeight > e.clientHeight || e.scrollWidth > e.clientWidth) out.push([e, 0, 0]);
    for (const p of out) {
      p[1] = p[0].scrollTop;
      p[2] = p[0].scrollLeft;
    }
    return out;
  }
  // Whether the page itself scrolls on an axis: the root's overflow, or the body's when the root's is
  // visible (the body's overflow then applies to the viewport).
  const pageScrolls = (prop) => {
    const own = getComputedStyle(document.documentElement)[prop];
    return !/hidden|clip/.test(own === "visible" ? getComputedStyle(document.body)[prop] : own);
  };
  // Null when a learner can scroll el into view: every ancestor that scrolls (overflow auto or scroll)
  // is scrolled to it, innermost first, and then no ancestor that does not scroll (overflow hidden or
  // clip) and, on a page that does not scroll, no viewport edge cuts it. Otherwise what cuts it.
  // The caller puts the scroll offsets back.
  function unreachable(el) {
    for (let e = el.parentElement; e && e !== document.documentElement; e = e.parentElement) {
      const cs = getComputedStyle(e);
      const r = el.getBoundingClientRect();
      const p = e.getBoundingClientRect();
      if (/auto|scroll/.test(cs.overflowY) && e.scrollHeight > e.clientHeight) {
        const top = p.top + e.clientTop;
        const bottom = top + e.clientHeight;
        if (r.bottom > bottom) e.scrollTop += Math.min(r.bottom - bottom, r.top - top);
        else if (r.top < top) e.scrollTop -= top - r.top;
      }
      if (/auto|scroll/.test(cs.overflowX) && e.scrollWidth > e.clientWidth) {
        const left = p.left + e.clientLeft;
        const right = left + e.clientWidth;
        if (r.right > right) e.scrollLeft += Math.min(r.right - right, r.left - left);
        else if (r.left < left) e.scrollLeft -= left - r.left;
      }
    }
    const r = el.getBoundingClientRect();
    const box = [Math.round(r.left), Math.round(r.top), Math.round(r.right), Math.round(r.bottom)];
    for (let e = el.parentElement; e && e !== document.documentElement; e = e.parentElement) {
      const cs = getComputedStyle(e);
      const p = e.getBoundingClientRect();
      if (/hidden|clip/.test(cs.overflowY) && (r.top < p.top - 1 || r.bottom > p.bottom + 1)) return { by: desc(e), axis: "y", box: box };
      if (/hidden|clip/.test(cs.overflowX) && (r.left < p.left - 1 || r.right > p.right + 1)) return { by: desc(e), axis: "x", box: box };
    }
    const vp = viewport();
    if (!pageScrolls("overflowY") && (r.top < -1 || r.bottom > vp.vh + 1)) return { by: "viewport", axis: "y", box: box };
    if (!pageScrolls("overflowX") && (r.left < -1 || r.right > vp.vw + 1)) return { by: "viewport", axis: "x", box: box };
    return null;
  }
  // Text outside the first viewport, measured in sections: scroll the first unmeasured element
  // into view, measure everything now in view, repeat. What no scroll can bring into view
  // (clipped away by a non-scrolling ancestor) has no composited background to sample.
  function belowTheFold(add, pending, vh) {
    if (!pending.length) return 0;
    const saved = scrollPositions();
    let measured = 0;
    while (pending.length) {
      const first = pending[0];
      first.scrollIntoView({ block: "start", inline: "nearest" });
      const rest = [];
      for (const el of pending) {
        if (inView(clipRect(el), vh)) {
          measured++;
          const control = controlText(el, getComputedStyle(el));
          if (control) controlContrast(add, el, control);
          else textContrast(add, el, ownText(el));
        } else if (el !== first) rest.push(el);
      }
      pending = rest;
    }
    for (const [e, top, left] of saved) {
      e.scrollTop = top;
      e.scrollLeft = left;
    }
    return measured;
  }

  // ---------- language ----------
  const LANG_EXEMPT = '[lang="en"],.exam,.pas-body,kbd,code,.mono,[data-raw],[translate="no"],input,textarea,[data-field]';
  const RAW_EXEMPT = 'kbd,code,.mono,[data-raw],[translate="no"],input,textarea,[data-field]';
  // Product, key and model-agnostic words a Chinese sentence may carry.
  const ZH_WORDS_OK = /^(Pass|Quest|PassQuest|Anki|Esc|Enter|Tab|Shift|Space|OK|AI)$/;
  const RAW_PATTERNS = [
    /\b[a-z]{2,}(_[a-z0-9]{2,})+\b/, // snake_case codes
    /\b[0-9a-f]{12,}\b/, // hashes
    /\bsha-?256\b/i,
    /\bpassquest\s+[a-z]/, // CLI commands
  ];

  // ---------- focus ----------
  function installFocusMirror() {
    if (document.getElementById("qa-fv")) return;
    const mirrored = [];
    const walk = (list) => {
      for (const r of list) {
        if (r.selectorText && r.selectorText.includes(":focus")) {
          const at = r.cssText.indexOf("{");
          const sel = r.cssText.slice(0, at).replace(/:focus-visible/g, ".qa-fv").replace(/:focus-within/g, ".qa-fw").replace(/:focus(?![-\w])/g, ".qa-fv");
          mirrored.push(sel + r.cssText.slice(at));
        } else if (r.cssRules && !(r.media && /reduced-motion/.test(r.media.mediaText))) walk(r.cssRules);
      }
    };
    for (const sheet of document.styleSheets) {
      try {
        walk(sheet.cssRules);
      } catch (error) {
        // A cross-origin sheet cannot be read; the app has none.
      }
    }
    const st = document.createElement("style");
    st.id = "qa-fv";
    st.textContent = mirrored.join("\n");
    document.head.append(st);
  }
  // What a focused control may visibly change. An outline counts only when it is drawn: an offset
  // or colour change under outline-style none is invisible.
  const FOCUS_PROPS = ["outline", "boxShadow", "borderTopColor", "backgroundColor", "color", "textDecorationLine"];
  const focusLook = (el) => {
    const cs = getComputedStyle(el);
    const drawn = cs.outlineStyle !== "none" && parseFloat(cs.outlineWidth) > 0;
    return FOCUS_PROPS.map((p) => (p === "outline" ? (drawn ? [cs.outlineStyle, cs.outlineWidth, cs.outlineColor, cs.outlineOffset].join(" ") : "none") : cs[p]));
  };
  function clearFocusClasses(el) {
    el.classList.remove("qa-fv");
    document.querySelectorAll(".qa-fw").forEach((p) => p.classList.remove("qa-fw"));
  }

  function audit(label) {
    const t0 = performance.now();
    collectPeLayers();
    installFocusMirror();
    const scope = root();
    const vis = Array.from(scope.el.querySelectorAll("*")).filter((e) => !SKIP_TAGS.has(e.tagName) && visible(e));
    const issues = [];
    const add = (check, el, extra) => issues.push(Object.assign({ check: check, el: path(el) }, extra || {}));
    const vp = viewport();
    const vw = vp.vw, vh = vp.vh;
    const lang = document.documentElement.lang;
    const de = document.documentElement;
    if (de.scrollWidth > de.clientWidth + 1) add("page-h-overflow", de, { sw: de.scrollWidth, cw: de.clientWidth });

    // What the state must show as it opens (the consent and budget a call strip names before a send):
    // measured before the pass scrolls anything into view.
    for (const el of vis.filter((e) => e.matches("[data-must-see]"))) {
      const hidden = hiddenPart(el);
      if (hidden) add("must-see-hidden", el, hidden);
    }

    // Text the viewport does not show yet: measured after the pass, scrolled into view.
    const unseen = [];
    for (const el of vis) {
      if (el.matches(".sr") || (el.offsetWidth <= 2 && el.offsetHeight <= 2)) continue;
      const cs = getComputedStyle(el);
      const text = ownText(el);
      const r = el.getBoundingClientRect();
      const control = controlText(el, cs);
      if (control) {
        if (inView(clipRect(el), vh)) controlContrast(add, el, control);
        else unseen.push(el);
      }
      if (text || (el.innerText && el.innerText.trim())) {
        if (cs.overflowX !== "visible" && !/auto|scroll/.test(cs.overflowX) && cs.textOverflow !== "ellipsis" && el.scrollWidth > el.clientWidth + 1 && el.clientWidth > 0) {
          const box = el.getBoundingClientRect();
          const culprit = Array.from(el.querySelectorAll("*")).filter((d) => visible(d) && d.getBoundingClientRect().right > box.right + 1).map((d) => desc(d) + " +" + Math.round(d.getBoundingClientRect().right - box.right) + "px").slice(-2);
          add("text-clipped-x", el, { sw: el.scrollWidth, cw: el.clientWidth, culprit: culprit.join(" "), text: (text || el.innerText).slice(0, 40) });
        }
        if (/hidden|clip/.test(cs.overflowY) && el.scrollHeight > el.clientHeight + 2 && el.clientHeight > 0 && text && cs.webkitLineClamp === "none") {
          add("text-clipped-y", el, { sh: el.scrollHeight, ch: el.clientHeight, text: text.slice(0, 40) });
        }
      }
      if ((text || el.matches(INTERACTIVE)) && (r.right > vw + 1 || r.left < -1)) {
        let scroller = false;
        for (let e = el.parentElement; e; e = e.parentElement) if (/auto|scroll/.test(getComputedStyle(e).overflowX) && e.scrollWidth > e.clientWidth) scroller = true;
        if (!scroller && clipRect(el).w > 0) add("outside-viewport-x", el, { left: Math.round(r.left), right: Math.round(r.right), vw: vw, text: text.slice(0, 40) });
      }
      for (const pe of ["::before", "::after"]) {
        const ps = getComputedStyle(el, pe);
        if (!/^["']|^counter|^attr/.test(ps.content) || /^["']\s*["']$/.test(ps.content) || ps.display === "none") continue;
        const pfs = parseFloat(ps.fontSize);
        if (pfs < 12) add("font-under-12px", el, { pseudo: pe, px: pfs, text: ps.content.slice(0, 20) });
        contrastFinding(add, el, contrast(el, ps.color, r, pfs, parseInt(ps.fontWeight, 10)), { pseudo: pe, px: pfs, text: ps.content.slice(0, 20) });
      }
      if (!text) continue;
      const c = clipRect(el);
      if (c.clippedBy && !c.clippedBy.scroll && c.w > 0 && c.h > 0 && (c.w < r.width - 2 || c.h < r.height - 2)) {
        add("text-cut-by-ancestor", el, { by: desc(c.clippedBy.el), axis: c.clippedBy.axis, shown: [Math.round(c.w), Math.round(c.h)], box: [Math.round(r.width), Math.round(r.height)], text: text.slice(0, 40) });
      }
      const fs = parseFloat(cs.fontSize);
      if (fs < 12) add("font-under-12px", el, { px: fs, text: text.slice(0, 40) });
      if (inView(c, vh)) textContrast(add, el, text);
      else unseen.push(el);
      if (/^zh/.test(lang) && !el.closest(LANG_EXEMPT)) {
        const words = (text.match(/[A-Za-z][A-Za-z'’-]{2,}/g) || []).filter((w) => !ZH_WORDS_OK.test(w));
        if (words.length) add("zh-ascii-words", el, { words: words.slice(0, 6).join(" "), text: text.slice(0, 50) });
      }
      if (lang === "en" && /[㐀-鿿]/.test(text) && !el.closest('[lang^="zh"],[data-raw],[translate="no"]')) add("en-cjk-text", el, { text: text.slice(0, 40) });
      if (RAW_PATTERNS.some((p) => p.test(text)) && !el.closest(RAW_EXEMPT)) add("raw-code", el, { text: text.slice(0, 60) });
    }
    // What the first viewport does not show must be reachable by a scroll the learner can make.
    if (unseen.length) {
      const saved = scrollPositions();
      for (const el of new Set(unseen)) {
        const cut = unreachable(el);
        if (cut) add("unreachable", el, Object.assign(cut, { text: (ownText(el) || el.innerText || el.getAttribute("aria-label") || "").trim().slice(0, 40) }));
      }
      for (const [e, top, left] of saved) {
        e.scrollTop = top;
        e.scrollLeft = left;
      }
    }
    const folded = belowTheFold(add, unseen, vh);

    // Scroll containers that scroll although their content fits: the content's boxes (with their
    // end margins) and text, plus the container's own end padding, measured from the scroll origin,
    // fit in the client box, so the overflow is decoration (such as a positioned pseudo-element)
    // and the scrollbar reveals nothing. A form control scrolls its own text; it is skipped.
    for (const el of vis) {
      if (el.matches("textarea,input,select")) continue;
      const cs = getComputedStyle(el);
      const sy = /auto|scroll/.test(cs.overflowY) && el.scrollHeight > el.clientHeight + 1;
      const sx = /auto|scroll/.test(cs.overflowX) && el.scrollWidth > el.clientWidth + 1;
      if (!sy && !sx) continue;
      const b = el.getBoundingClientRect();
      const top = b.top + el.clientTop - el.scrollTop;
      const left = b.left + el.clientLeft - el.scrollLeft;
      let needY = 0;
      let needX = 0;
      const reach = (r, mb, mr) => {
        if (r.width + r.height === 0) return;
        needY = Math.max(needY, r.bottom + mb - top);
        needX = Math.max(needX, r.right + mr - left);
      };
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT);
      const range = document.createRange();
      for (let n = walker.nextNode(); n; n = walker.nextNode()) {
        if (n.nodeType === 3) {
          if (!n.textContent.trim()) continue;
          range.selectNodeContents(n);
          reach(range.getBoundingClientRect(), 0, 0);
        } else if (!SKIP_TAGS.has(n.tagName) && visible(n)) {
          const ns = getComputedStyle(n);
          reach(n.getBoundingClientRect(), Math.max(0, parseFloat(ns.marginBottom) || 0), Math.max(0, parseFloat(ns.marginRight) || 0));
        }
      }
      needY += parseFloat(cs.paddingBottom);
      needX += parseFloat(cs.paddingRight);
      if (sy && needY <= el.clientHeight + 1) add("scroll-without-content", el, { axis: "y", sh: el.scrollHeight, ch: el.clientHeight, content: Math.round(needY) });
      else if (sx && needX <= el.clientWidth + 1) add("scroll-without-content", el, { axis: "x", sw: el.scrollWidth, cw: el.clientWidth, content: Math.round(needX) });
    }

    // Controls: the centre of the visible part must hit the control itself.
    for (const el of vis.filter((e) => e.matches(INTERACTIVE))) {
      const c = clipRect(el);
      if ((c.w >= vw * 0.9 && c.h >= vh * 0.9) || c.w < 2 || c.h < 2) continue; // a scrim, or clipped away
      const x = (c.l + c.r) / 2;
      const y = (c.t + c.b) / 2;
      if (x < 0 || y < 0 || x > vw || y > vh) {
        if (!c.clippedBy) add("interactive-offscreen", el, { x: Math.round(x), y: Math.round(y) });
        continue;
      }
      const hit = document.elementFromPoint(x, y);
      if (!hit || hit === el || el.contains(hit)) continue;
      if (hit.contains(el)) {
        if (getComputedStyle(el).pointerEvents !== "none") add("interactive-not-hit", el, { hit: desc(hit) });
      } else add("interactive-covered", el, { by: path(hit) });
    }

    // Keyboard: every control focusable (a scrim is pointer-only; Esc closes its layer) with a visible change.
    const reachable = [];
    for (const el of vis.filter((e) => e.matches(CONTROLS))) {
      if (el.matches(".scrim") || el.disabled) continue;
      const target = focusable(el) ? el : el.querySelector("button,a[href],input,select,textarea,[tabindex]:not([tabindex='-1'])");
      if (target && target.disabled) continue;
      if (!target || !focusable(target)) {
        add("not-tabbable", el, { text: (el.innerText || el.getAttribute("aria-label") || "").trim().slice(0, 30) });
        continue;
      }
      if (!reachable.includes(target)) reachable.push(target);
    }
    const before = document.activeElement;
    for (const el of reachable) {
      if (document.activeElement === el) el.blur();
      const b = focusLook(el);
      el.focus({ preventScroll: true });
      if (document.activeElement !== el) {
        add("focus-refused", el, {});
        continue;
      }
      el.classList.add("qa-fv");
      for (let p = el.parentElement; p; p = p.parentElement) p.classList.add("qa-fw");
      const after = focusLook(el);
      const changed = FOCUS_PROPS.filter((p, i) => after[i] !== b[i]);
      if (!changed.length) add("no-focus-indicator", el, { text: (el.innerText || el.getAttribute("aria-label") || "").trim().slice(0, 30) });
      clearFocusClasses(el);
      el.blur();
    }
    if (before && before.focus && before !== document.body) before.focus({ preventScroll: true });
    if (scope.modal) {
      const outside = Array.from(document.querySelectorAll("button,a[href],input,select,textarea,[tabindex]")).filter((e) => !scope.el.contains(e) && focusable(e) && visible(e));
      if (outside.length) add("modal-not-trapping", scope.el, { outside: outside.slice(0, 3).map(desc).join(" ") });
    }
    return { label: label, issues: issues, folded: folded, ms: Math.round(performance.now() - t0) };
  }

  // Null when el is wholly inside the viewport and every clipping ancestor (so no scroll container
  // hides any of it); otherwise where it is and how much of it shows.
  function hiddenPart(el) {
    const r = el.getBoundingClientRect();
    const c = clipRect(el);
    const vp = viewport();
    const inside = c.w >= r.width - 1 && c.h >= r.height - 1 && r.left >= -1 && r.top >= -1 && r.right <= vp.vw + 1 && r.bottom <= vp.vh + 1;
    if (inside) return null;
    return { box: [Math.round(r.left), Math.round(r.top), Math.round(r.right), Math.round(r.bottom)], shown: [Math.round(Math.max(c.w, 0)), Math.round(Math.max(c.h, 0))], by: c.clippedBy ? desc(c.clippedBy.el) : "viewport" };
  }

  // The state's primary action: exactly one visible [data-primary] in the audited root, wholly
  // inside the viewport and every scroll container, and hit by a click at its centre.
  function primary(scopeEl) {
    const scope = scopeEl || root().el;
    const marked = Array.from(scope.querySelectorAll("[data-primary]")).filter(visible);
    const where = scope === document.body ? "page" : "#" + scope.id;
    if (marked.length === 0) return [{ check: "primary-missing", el: where }];
    if (marked.length > 1) return [{ check: "primary-ambiguous", el: where, marked: marked.map(desc).join(" ") }];
    const el = marked[0];
    const r = el.getBoundingClientRect();
    const hidden = hiddenPart(el);
    if (hidden) return [Object.assign({ check: "primary-hidden", el: path(el) }, hidden)];
    const hit = document.elementFromPoint((r.left + r.right) / 2, (r.top + r.bottom) / 2);
    if (hit && hit !== el && !el.contains(hit)) return [{ check: "primary-covered", el: path(el), by: path(hit) }];
    return [];
  }

  // ---------- motion ----------
  // The @media (prefers-reduced-motion: reduce) rules as the page shipped them.
  const reducedRules = [];
  const collectReduced = (list) => {
    for (const r of list) {
      if (r.media && /prefers-reduced-motion:\s*reduce/.test(r.media.mediaText)) reducedRules.push(r);
      else if (r.cssRules) collectReduced(r.cssRules);
    }
  };
  for (const sheet of document.styleSheets) {
    try {
      collectReduced(sheet.cssRules);
    } catch (error) {
      // A cross-origin sheet cannot be read; the app has none.
    }
  }
  const reducedText = reducedRules.map((r) => Array.from(r.cssRules, (x) => x.cssText).join("\n")).join("\n");
  const realMatchMedia = window.matchMedia.bind(window);
  const answerMotion = (reduce) => {
    window.matchMedia = (query) =>
      /prefers-reduced-motion/.test(query)
        ? { matches: /reduce/.test(query) === reduce, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }
        : realMatchMedia(query);
  };
  // Reduced motion whatever the platform says: the rules apply unconditionally and matchMedia agrees.
  function emulateReducedMotion() {
    let st = document.getElementById("qa-reduced");
    if (!st) {
      st = document.createElement("style");
      st.id = "qa-reduced";
      document.head.append(st);
    }
    st.textContent = reducedText;
    answerMotion(true);
    return reducedRules.length;
  }
  // No motion preference whatever the platform says: the reduce rules are removed.
  function emulateNoPreference() {
    const st = document.getElementById("qa-reduced");
    if (st) st.remove();
    for (const r of reducedRules) {
      const parent = r.parentRule || r.parentStyleSheet;
      const index = parent ? Array.from(parent.cssRules).indexOf(r) : -1;
      if (index >= 0) parent.deleteRule(index);
    }
    answerMotion(false);
    return reducedRules.length;
  }
  function restoreMotion() {
    const st = document.getElementById("qa-reduced");
    if (st) st.remove();
    window.matchMedia = realMatchMedia;
  }
  // Running animations and transitions longer than 1 ms or looping forever.
  function motionReport(label) {
    const out = [];
    for (const a of document.getAnimations()) {
      if (a.playState !== "running") continue;
      const t = a.effect && a.effect.getComputedTiming ? a.effect.getComputedTiming() : {};
      if ((t.duration || 0) > 1 || t.iterations === Infinity) {
        out.push({ check: "motion-under-reduce", el: path(a.effect && a.effect.target), at: label, name: a.animationName || a.transitionProperty || a.constructor.name, ms: t.duration, iterations: t.iterations });
      }
    }
    return out;
  }
  // Every CSS transition that exists now (pending or running), however short. Called in the same
  // task as a style change: reading the animations flushes style, which is when transitions start.
  function transitionReport(label) {
    return document
      .getAnimations()
      .filter((a) => a instanceof CSSTransition)
      .map((a) => ({ check: "transition-under-reduce", el: path(a.effect && a.effect.target), at: label, name: a.transitionProperty, ms: a.effect.getComputedTiming().duration }));
  }
  // Running transitions of the room colours (the .atmos custom properties or the page background),
  // leaving out the animations in except (those that were running before a condition change).
  function roomTransitions(except = new Set()) {
    return document.getAnimations().filter((a) => !except.has(a) && a.transitionProperty && /^(--a[123]|background(-color)?)$/.test(a.transitionProperty) && a.effect.getComputedTiming().duration > 1).map((a) => a.transitionProperty + " " + a.effect.getComputedTiming().duration + "ms");
  }

  // ---------- planted defects ----------
  // One planted defect per check on the live page (in zh); every check must report it.
  async function probes(host) {
    const box = document.createElement("div");
    box.id = "qa-probes";
    box.innerHTML =
      '<div id="qa-clip" style="width:80px;overflow:hidden;white-space:nowrap">QA clipped sentence that cannot fit in eighty pixels</div>' +
      '<p id="qa-low" style="color:rgb(150,150,150);background:#fff">QA low contrast text</p>' +
      '<div style="height:40px;overflow:auto"><div style="height:400px"></div><p id="qa-low-fold" style="color:rgb(150,150,150);background:#fff">QA low contrast below the fold</p></div>' +
      '<input id="qa-low-input" style="color:rgb(150,150,150);background:#fff" value="QA faint value">' +
      '<p id="qa-tiny" style="font-size:10px">QA tiny text</p>' +
      '<span id="qa-div-act" data-act="qa-noop">QA div control</span>' +
      '<button id="qa-nofocus" class="qa-nofocus">QA no ring</button>' +
      '<button id="qa-under" style="position:relative">QA covered</button><div style="position:relative;margin-top:-30px;height:40px;background:#fff">QA cover</div>' +
      '<p id="qa-raw">错误 provider_auth_failed 发生</p>' +
      '<div id="qa-stray" style="position:relative;height:40px;overflow:auto">QA stray scrollbar</div>' +
      '<div id="qa-scroll" style="height:40px;overflow:auto"><div style="height:200px"></div><button id="qa-primary" data-primary>QA primary</button></div>' +
      '<div style="height:40px;overflow:auto"><div style="height:200px"></div><p id="qa-must-see" data-must-see>QA consent row</p></div>' +
      '<div style="height:40px;overflow:hidden"><div style="height:200px"></div><p id="qa-unreachable">QA text no scroll reaches</p></div>' +
      '<i id="qa-spin" style="display:inline-block;animation:qa-spin 2s linear infinite !important">QA spin</i>' +
      '<b id="qa-fade" style="transition:color 2s !important">QA fade</b>';
    const wide = document.createElement("div");
    wide.id = "qa-wide";
    wide.style.cssText = "position:fixed;top:300px;left:" + (innerWidth - 40) + "px;width:200px";
    wide.textContent = "QA off-screen text";
    const st = document.createElement("style");
    st.id = "qa-probe-style";
    st.textContent = "@keyframes qa-spin{to{transform:rotate(360deg)}} .qa-nofocus:focus-visible,.qa-nofocus:focus{outline:none!important;box-shadow:none!important} #qa-stray::after{content:'';position:absolute;left:0;top:100%;width:10px;height:30px}";
    document.head.append(st);
    host.append(box);
    document.body.append(wide);
    const mirror = document.getElementById("qa-fv");
    if (mirror) mirror.remove(); // mirror the probe's own focus rule too
    emulateReducedMotion();
    await new Promise((done) => setTimeout(done, 100));
    const found = audit("probes").issues.concat(primary(box));
    const motion = motionReport("probes");
    document.getElementById("qa-fade").style.color = "rgb(200, 0, 0)";
    const transitions = transitionReport("probes");
    // The id itself, not a longer id it starts (#qa-low is not #qa-low-fold).
    const fired = (check, id) => found.some((i) => i.check === check && i.el.split("#" + id).slice(1).some((rest) => !/^[-_a-zA-Z0-9]/.test(rest)));
    const result = {
      "text-clipped-x": fired("text-clipped-x", "qa-clip"),
      "outside-viewport-x": fired("outside-viewport-x", "qa-wide"),
      "contrast-fail": fired("contrast-fail", "qa-low"),
      "contrast-fail below the fold": fired("contrast-fail", "qa-low-fold"),
      "contrast-fail on a control's value": fired("contrast-fail", "qa-low-input"),
      "font-under-12px": fired("font-under-12px", "qa-tiny"),
      "not-tabbable": fired("not-tabbable", "qa-div-act"),
      "no-focus-indicator": fired("no-focus-indicator", "qa-nofocus"),
      "interactive-covered": fired("interactive-covered", "qa-under"),
      "zh-ascii-words": fired("zh-ascii-words", "qa-raw"),
      "raw-code": fired("raw-code", "qa-raw"),
      "primary-hidden": fired("primary-hidden", "qa-primary"),
      "must-see-hidden": fired("must-see-hidden", "qa-must-see"),
      "scroll-without-content": fired("scroll-without-content", "qa-stray"),
      unreachable: fired("unreachable", "qa-unreachable"),
      "motion-under-reduce": motion.some((m) => m.el.includes("#qa-spin")),
      "transition-under-reduce": transitions.some((m) => m.el.includes("#qa-fade")),
    };
    box.remove();
    wide.remove();
    st.remove();
    const stale = document.getElementById("qa-fv");
    if (stale) stale.remove();
    restoreMotion();
    return result;
  }

  return { audit: audit, primary: primary, hidden: hiddenPart, path: path, desc: desc, visible: visible, emulateReducedMotion: emulateReducedMotion, emulateNoPreference: emulateNoPreference, restoreMotion: restoreMotion, motionReport: motionReport, transitionReport: transitionReport, roomTransitions: roomTransitions, probes: probes };
})();
`;
