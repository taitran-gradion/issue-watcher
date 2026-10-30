import { api } from "@margince/frontend/api";
import { currentPage, patternOf } from "./route";

// The watcher: started once, when the app loads the extension, so it sees the
// page's first requests. It turns what goes wrong into issues, shows them to
// the member at once, and reports each to the server for developers.

export type Kind = "server" | "refused" | "network" | "app" | "slow" | "stuck" | "ui";

export type Config = {
  server_errors: boolean;
  refusals: boolean;
  app_errors: boolean;
  ui_problems: boolean;
  slow_ms: number;
  stuck_ms: number;
  toasts: boolean;
  ignore: string[];
};

export const defaultConfig: Config = {
  server_errors: true,
  refusals: true,
  app_errors: true,
  ui_problems: true,
  slow_ms: 3000,
  stuck_ms: 10000,
  toasts: true,
  ignore: [],
};

// One issue as this browser saw it.
export type Seen = { id: number; kind: Kind; title: string; detail: string; page: string; reference: string; at: number };

let config = defaultConfig;
// Signed out, the app's own complaints about it are not issues: the watcher
// stays quiet while the app's session check says nobody is signed in. It starts
// quiet, since the app may ask before the watcher is running.
let signedOut = true;
let issues: Seen[] = [];
let unseen = 0;
let nextId = 1;
const listeners = new Set<() => void>();
const maxKept = 50;

export function setConfig(next: Config) {
  config = next;
}

// The store the watcher's button and panel read.
export const store = {
  signedOut: () => signedOut,
  subscribe(listener: () => void) {
    listeners.add(listener);
    return () => listeners.delete(listener);
  },
  issues: () => issues,
  unseen: () => unseen,
  markSeen() {
    unseen = 0;
    notify();
  },
  clear() {
    issues = [];
    unseen = 0;
    notify();
  },
  toasts: () => config.toasts,
};

function notify() {
  for (const listener of listeners) listener();
}

// Requests the extension sends itself, and the sticky note's, are not the app's.
const ours = (path: string) => /\/ext\/(issue-watch|sticky-note)\//.test(path);

// The same problem is reported once a minute at most, so a request failing in
// a loop is one issue counting up, not a flood.
const lastReported = new Map<string, number>();
const reportEvery = 60_000;

function emit(kind: Kind, title: string, detail = "", reference = "") {
  if (signedOut) return;
  const text = `${title} ${detail}`.toLowerCase();
  if (config.ignore.some((ignored) => ignored && text.includes(ignored.toLowerCase()))) return;
  const page = currentPage();
  const key = `${kind}|${patternOf(page)}|${title.replace(/[0-9a-f-]{36}|\d{4,}/gi, ":id")}`;
  const now = Date.now();
  if (now - (lastReported.get(key) ?? 0) < reportEvery) return;
  lastReported.set(key, now);
  issues = [{ id: nextId++, kind, title, detail, page, reference, at: now }, ...issues].slice(0, maxKept);
  unseen++;
  notify();
  void api
    .POST("/ext/issue-watch/report", {
      body: {
        kind,
        title: title.slice(0, 300),
        detail: detail.slice(0, 2000),
        pattern: patternOf(page),
        page,
        reference: reference.slice(0, 300),
        browser: navigator.userAgent.slice(0, 300),
      },
    })
    // A member whose role may not report still sees the issue; nothing else to do.
    .catch(() => undefined);
}

// What a problem+json answer says about itself: its words and its reference.
async function problemOf(response: Response): Promise<{ detail: string; reference: string }> {
  if (!(response.headers.get("content-type") ?? "").includes("json")) return { detail: "", reference: "" };
  try {
    const body: unknown = await response.clone().json();
    if (typeof body !== "object" || body === null) return { detail: "", reference: "" };
    const field = (name: string) => {
      const value = (body as Record<string, unknown>)[name];
      return typeof value === "string" ? value : "";
    };
    return {
      detail: [field("title"), field("detail")].filter(Boolean).join(": "),
      reference: field("instance") || field("code"),
    };
  } catch {
    return { detail: "", reference: "" };
  }
}

function watchRequests() {
  const fetchOriginal = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    // Read the method and address without copying the request: a copy uses up
    // its body, and the app's own fetch would then fail on every save.
    const url = new URL(input instanceof Request ? input.url : String(input), window.location.href);
    const method = (init?.method ?? (input instanceof Request ? input.method : "GET")).toUpperCase();
    if (url.origin !== window.location.origin || ours(url.pathname)) return fetchOriginal(input, init);
    const label = `${method} ${url.pathname}`;
    const started = performance.now();
    let response: Response;
    try {
      response = await fetchOriginal(input, init);
    } catch (error) {
      // A request the page itself called off is not a problem.
      if (!(error instanceof DOMException && error.name === "AbortError") && config.server_errors) {
        emit("network", `${label} did not reach the server`, error instanceof Error ? error.message : "");
      }
      throw error;
    }
    const took = performance.now() - started;
    const status = response.status;
    // Who is signed in is what the app's session check says.
    if (url.pathname === "/v1/me" && (status === 401 || response.ok)) {
      const wasSignedOut = signedOut;
      signedOut = status === 401;
      if (signedOut !== wasSignedOut) notify();
    }
    if (status >= 500 && config.server_errors) {
      const problem = await problemOf(response);
      emit("server", `${label} ${status}`, problem.detail, problem.reference);
    } else if (status >= 400 && status !== 401 && status !== 404 && config.refusals) {
      const problem = await problemOf(response);
      emit("refused", `${label} ${status}`, problem.detail, problem.reference);
    } else if (config.slow_ms > 0 && took > config.slow_ms) {
      emit("slow", `${label} took ${(took / 1000).toFixed(1)}s`);
    }
    return response;
  };
}

const textOf = (value: unknown) => (value instanceof Error ? value.message : String(value));

function watchErrors() {
  window.addEventListener("error", (event) => {
    if (config.app_errors) emit("app", textOf(event.error ?? event.message).slice(0, 300), event.filename ? `${event.filename}:${event.lineno}` : "");
  });
  window.addEventListener("unhandledrejection", (event) => {
    if (config.app_errors) emit("app", textOf(event.reason).slice(0, 300));
  });
  const consoleError = console.error.bind(console);
  console.error = (...args: unknown[]) => {
    consoleError(...args);
    if (config.app_errors) emit("app", args.map(textOf).join(" ").slice(0, 300));
  };
}

// Loading indicators: a spinner still showing after stuck_ms is stuck.
const busy = '[aria-busy="true"], [role="progressbar"]';

function watchSpinners() {
  const since = new WeakMap<Element, number>();
  const reported = new WeakSet<Element>();
  window.setInterval(() => {
    if (config.stuck_ms <= 0) return;
    const now = Date.now();
    for (const el of document.querySelectorAll(busy)) {
      if (el.closest(ourHosts)) continue;
      const box = el.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) {
        since.delete(el);
        continue;
      }
      const first = since.get(el) ?? now;
      since.set(el, first);
      if (now - first > config.stuck_ms && !reported.has(el)) {
        reported.add(el);
        const name = el.getAttribute("aria-label") || el.textContent?.trim().slice(0, 60) || el.tagName.toLowerCase();
        emit("stuck", `Loading "${name}" still showing after ${Math.round((now - first) / 1000)}s`);
      }
    }
  }, 1000);
}

// What the member sees that is wrong even when every request worked: an error
// on screen, text the app never meant to show, a picture that did not load, a
// page gone blank or wider than the window.

const ourHosts = "#ext-issue-watch-host, #ext-sticky-note-host";
// Words a value turns into when it was never there.
const brokenWords = /\b(undefined|NaN|Invalid Date)\b|\[object Object\]/;
// A translation key shown as itself: dotted words, one with a capital, the way
// a missing translation reads ("deals.stageTitle"). A web address has no capital.
const rawKey = /^[a-z][a-zA-Z0-9]*(\.[a-zA-Z0-9_]+)+$/;

const visible = (el: Element) => {
  const box = el.getBoundingClientRect();
  return box.width > 0 && box.height > 0;
};

// Where on the page an element sits, in words a developer can search for.
function whereOf(el: Element): string {
  const parts: string[] = [];
  for (let at: Element | null = el; at && at !== document.body && parts.length < 3; at = at.parentElement) {
    const name = at.getAttribute("aria-label");
    const cls = typeof at.className === "string" ? at.className.trim().split(/\s+/)[0] : "";
    parts.unshift(name ? `${at.tagName.toLowerCase()}[${name}]` : cls ? `${at.tagName.toLowerCase()}.${cls}` : at.tagName.toLowerCase());
  }
  return parts.join(" > ");
}

const short = (text: string) => text.replace(/\s+/g, " ").trim().slice(0, 120);

function scanScreen(reported: WeakSet<Node>) {
  const main = document.querySelector("main") ?? document.body;
  // An error the app shows: the danger notice, or anything announced as an alert.
  for (const el of document.querySelectorAll('[role="alert"], .callout-danger')) {
    if (reported.has(el) || el.closest(ourHosts) || !visible(el)) continue;
    const text = short(el.textContent ?? "");
    if (!text) continue;
    reported.add(el);
    emit("ui", `Error shown: "${text}"`, whereOf(el));
  }
  // Text the app never meant to show.
  const walker = document.createTreeWalker(main, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const text = node.textContent ?? "";
    const parent = node.parentElement;
    if (!parent || reported.has(node) || text.trim().length === 0) continue;
    if (parent.closest(`${ourHosts}, code, pre, script, style, textarea, [contenteditable="true"]`)) continue;
    const trimmed = text.trim();
    const shown = brokenWords.exec(text)?.[0] ?? (rawKey.test(trimmed) && /[A-Z]/.test(trimmed) ? trimmed : "");
    if (!shown || !visible(parent)) continue;
    reported.add(node);
    emit("ui", shown === trimmed ? `Untranslated text "${shown}"` : `Shows "${shown}" in "${short(text)}"`, whereOf(parent));
  }
  // Pictures that did not load.
  for (const img of document.querySelectorAll("img")) {
    if (reported.has(img) || !img.complete || img.naturalWidth > 0 || !img.getAttribute("src") || img.closest(ourHosts)) continue;
    reported.add(img);
    emit("ui", `Picture did not load: ${short(img.alt || img.src)}`, `${img.src.slice(0, 300)} at ${whereOf(img)}`);
  }
  // The page wider than its space: something pushes it sideways. The app's page
  // area scrolls by itself, so it is measured as well as the window.
  const over = Math.max(
    document.documentElement.scrollWidth - document.documentElement.clientWidth,
    main.scrollWidth - main.clientWidth,
  );
  if (over > 1) {
    const edge = Math.min(window.innerWidth, main.getBoundingClientRect().right);
    const culprit = [...main.querySelectorAll("*")].find((el) => el.getBoundingClientRect().right > edge + 1 && !el.closest(ourHosts));
    emit("ui", `Page is ${over}px wider than its space`, culprit ? whereOf(culprit) : "");
  }
}

function watchScreen() {
  const reported = new WeakSet<Node>();
  // Scanning waits for the page to change, and then for it to settle.
  let changed = true;
  let settledAt = 0;
  new MutationObserver(() => {
    changed = true;
    settledAt = Date.now() + 500;
  }).observe(document.body, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ["role", "class", "src"] });
  // A page with nothing in it, while nothing is loading, is blank.
  let blankSince = 0;
  let blankReported = "";
  window.setInterval(() => {
    if (!config.ui_problems) return;
    const main = document.querySelector("main");
    const page = currentPage();
    const empty = main !== null && visible(main) && (main.textContent ?? "").trim() === "" && !main.querySelector("img, svg, canvas, input, iframe") && !document.querySelector(busy);
    blankSince = empty ? blankSince || Date.now() : 0;
    if (empty && Date.now() - blankSince > 5000 && blankReported !== page) {
      blankReported = page;
      emit("ui", "Page is blank", "Nothing showed in the page for 5s, and nothing was loading.");
    }
    if (!empty && blankReported === page) blankReported = "";
    if (!changed || Date.now() < settledAt) return;
    changed = false;
    scanScreen(reported);
  }, 1000);
}

export function startMonitor() {
  watchRequests();
  watchErrors();
  watchSpinners();
  watchScreen();
}
