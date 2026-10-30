import { useCan, useT } from "@margince/frontend/app";
import {
  Badge,
  IconAction,
  useClipboardCopy,
} from "@margince/frontend/design-system";
import {
  Activity,
  Bug,
  Copy,
  ExternalLink,
  List,
  Trash2,
  X,
} from "lucide-react";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useConfig, useScreenGithub } from "./api";
import { type Kind, type Seen, setConfig, store } from "./monitor";
import { patternOf, usePage } from "./route";

const toastFor = 8000;
const maxToasts = 3;

export const kindTone: Record<Kind, "danger" | "warning" | "info"> = {
  server: "danger",
  network: "danger",
  app: "danger",
  refused: "warning",
  slow: "info",
  stuck: "warning",
  ui: "warning",
};

function useStore() {
  const issues = useSyncExternalStore(store.subscribe, store.issues);
  const unseen = useSyncExternalStore(store.subscribe, store.unseen);
  const signedOut = useSyncExternalStore(store.subscribe, store.signedOut);
  return { issues, unseen, signedOut };
}

// The watcher on every page: a button that turns red with the number of new
// issues, a toast as each one happens, and a panel with what went wrong.
export function Watcher() {
  const t = useT();
  const config = useConfig();
  const { issues, unseen, signedOut } = useStore();
  const [open, setOpen] = useState(false);
  const [githubOpen, setGithubOpen] = useState(false);
  const page = usePage();
  const canList = useCan("ext_issue_watch_issue", "read");
  const github = useScreenGithub(page, canList && !signedOut);
  const [toasts, setToasts] = useState<(Seen & { until: number })[]>([]);
  const lastShown = useRef(0);

  // The admin's settings reach the monitor as soon as they are read.
  useEffect(() => {
    if (config.data) setConfig(config.data);
  }, [config.data]);

  // Each new issue shows as a toast for a few seconds, unless toasts are off.
  useEffect(() => {
    const fresh = issues.filter((issue) => issue.id > lastShown.current);
    if (fresh.length === 0) return;
    lastShown.current = fresh[0]?.id ?? lastShown.current;
    if (open || !store.toasts()) return;
    const until = Date.now() + toastFor;
    setToasts((was) =>
      [...fresh.map((issue) => ({ ...issue, until })), ...was].slice(
        0,
        maxToasts,
      ),
    );
  }, [issues, open]);

  // A toast goes when its time is up.
  useEffect(() => {
    if (toasts.length === 0) return;
    const next = Math.min(...toasts.map((toast) => toast.until)) - Date.now();
    const timer = window.setTimeout(
      () => setToasts((was) => was.filter((toast) => toast.until > Date.now())),
      Math.max(0, next) + 50,
    );
    return () => window.clearTimeout(timer);
  }, [toasts]);

  useEffect(() => {
    if (open) store.markSeen();
  }, [open, issues]);

  if (signedOut) return null;
  const label =
    unseen > 0
      ? t("extIssueWatch.button.some", { count: String(unseen) })
      : t("extIssueWatch.button.none");
  return (
    <div className="ext-issue-watch">
      {!open && toasts.length > 0 && (
        <div className="ext-issue-watch-toasts" role="status">
          {toasts.map((toast) => (
            <button
              type="button"
              key={toast.id}
              className={`ext-issue-watch-toast is-${kindTone[toast.kind]}`}
              onClick={() => {
                setToasts([]);
                setOpen(true);
              }}
            >
              <strong>{t(`extIssueWatch.kind.${toast.kind}`)}</strong>
              <span>{toast.title}</span>
            </button>
          ))}
        </div>
      )}
      {githubOpen && github.data && (
        <GithubPanel known={github.data} onClose={() => setGithubOpen(false)} />
      )}
      {!open && !githubOpen && github.data && github.data.total > 0 && (
        <button
          type="button"
          className="ext-issue-watch-github"
          title={t("extIssueWatch.github.button", {
            count: String(github.data.total),
            screen: github.data.screen,
          })}
          aria-label={t("extIssueWatch.github.button", {
            count: String(github.data.total),
            screen: github.data.screen,
          })}
          onClick={() => setGithubOpen(true)}
        >
          <Bug size={14} aria-hidden />
          {String(github.data.total)}
        </button>
      )}
      {open ? (
        <IssuePanel issues={issues} onClose={() => setOpen(false)} />
      ) : (
        <button
          type="button"
          className={`ext-issue-watch-fab${unseen > 0 ? " has-issues" : ""}`}
          title={label}
          aria-label={label}
          onClick={() => {
            setToasts([]);
            setOpen(true);
          }}
        >
          <Activity size={22} aria-hidden />
          {unseen > 0 && (
            <span className="ext-issue-watch-badge" aria-hidden>
              <Badge variant="primary" tone="danger">
                {String(unseen)}
              </Badge>
            </span>
          )}
        </button>
      )}
    </div>
  );
}

function IssuePanel({
  issues,
  onClose,
}: Readonly<{ issues: Seen[]; onClose: () => void }>) {
  const t = useT();
  const canList = useCan("ext_issue_watch_issue", "read");
  const time = new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
  return (
    <section
      className="ext-issue-watch-panel"
      aria-label={t("extIssueWatch.panel.title")}
    >
      <div className="ext-issue-watch-head">
        <strong>{t("extIssueWatch.panel.title")}</strong>
        <div className="ext-issue-watch-tools">
          {canList && (
            <IconAction
              label={t("extIssueWatch.panel.all")}
              icon={<List size={16} aria-hidden />}
              href="#/ext/issue-watch"
            />
          )}
          <IconAction
            label={t("extIssueWatch.panel.clear")}
            icon={<Trash2 size={16} aria-hidden />}
            disabled={issues.length === 0}
            onClick={() => store.clear()}
          />
          <IconAction
            label={t("extIssueWatch.close")}
            icon={<X size={16} aria-hidden />}
            onClick={onClose}
          />
        </div>
      </div>
      {issues.length === 0 ? (
        <p className="t-meta">{t("extIssueWatch.panel.none")}</p>
      ) : (
        <ul className="ext-issue-watch-list">
          {issues.map((issue) => (
            <li
              key={issue.id}
              className={`ext-issue-watch-item is-${kindTone[issue.kind]}`}
            >
              <div className="ext-issue-watch-item-head">
                <Badge tone={kindTone[issue.kind]}>
                  {t(`extIssueWatch.kind.${issue.kind}`)}
                </Badge>
                <span
                  className="t-meta ext-issue-watch-where"
                  title={issue.page}
                >
                  {time.format(issue.at)} · {patternOf(issue.page)}
                </span>
              </div>
              <code className="ext-issue-watch-title">{issue.title}</code>
              {issue.detail && <p className="t-meta">{issue.detail}</p>}
              {issue.reference && <Reference value={issue.reference} />}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

type Known = NonNullable<ReturnType<typeof useScreenGithub>["data"]>;

// The tracker's open issues about this screen, each with why it is here: the
// match is by text, so the reason lets a developer judge it.
function GithubPanel({
  known,
  onClose,
}: Readonly<{ known: Known; onClose: () => void }>) {
  const t = useT();
  const synced = known.synced_at
    ? new Intl.DateTimeFormat(undefined, {
        dateStyle: "short",
        timeStyle: "short",
      }).format(new Date(known.synced_at))
    : "";
  return (
    <section
      className="ext-issue-watch-panel"
      aria-label={t("extIssueWatch.github.title")}
    >
      <div className="ext-issue-watch-head">
        <strong>{t("extIssueWatch.github.title")}</strong>
        <div className="ext-issue-watch-tools">
          <IconAction
            label={t("extIssueWatch.close")}
            icon={<X size={16} aria-hidden />}
            onClick={onClose}
          />
        </div>
      </div>
      <p className="t-meta">
        {t("extIssueWatch.github.sub", {
          count: String(known.total),
          open: String(known.open),
          screen: known.screen,
          repo: known.repo,
        })}
        {synced && ` · ${t("extIssueWatch.github.synced", { when: synced })}`}
      </p>
      {known.sync_error && (
        <p className="t-meta">
          {t("extIssueWatch.github.syncFailed", { error: known.sync_error })}
        </p>
      )}
      <ul className="ext-issue-watch-list">
        {known.issues.map((issue) => (
          <li key={issue.number} className="ext-issue-watch-item">
            <a
              className="ext-issue-watch-link"
              href={issue.url}
              target="_blank"
              rel="noreferrer"
            >
              #{issue.number} {issue.title}
              <ExternalLink size={12} aria-hidden />
            </a>
            <span className="t-meta">{issue.why}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

// The server's own reference for a failure, which finds it in the API log.
export function Reference({ value }: Readonly<{ value: string }>) {
  const t = useT();
  const copy = useClipboardCopy(value, {
    copy: t("extIssueWatch.copy"),
    copied: t("extIssueWatch.copied"),
    remedy: t("extIssueWatch.copyRemedy"),
  });
  return (
    <span className="ext-issue-watch-reference t-meta">
      {t("extIssueWatch.reference")} <code>{value}</code>
      <IconAction
        label={copy.label}
        icon={<Copy size={12} aria-hidden />}
        inline
        onClick={copy.copy}
      />
      {copy.notice}
    </span>
  );
}
