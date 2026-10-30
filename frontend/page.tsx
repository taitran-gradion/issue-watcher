import { problemMessageOf } from "@margince/frontend/api";
import { useCan, useLocale, useT } from "@margince/frontend/app";
import {
  Badge,
  Button,
  Callout,
  Card,
  DataTable,
  EmptyState,
  Field,
  Row,
  SectionHeader,
  SegmentedControl,
  TextInput,
} from "@margince/frontend/design-system";
import { useEffect, useState } from "react";
import { type Status, statusOf, useConfig, useIssues, useResolve, useSaveConfig } from "./api";
import type { Config } from "./monitor";
import { kindTone, Reference } from "./watcher";

const statuses = ["open", "resolved", "all"] as const;

// #/ext/issue-watch: every issue members' browsers ran into, the most recently
// seen first, and what counts as one.
export function IssuesPage() {
  const t = useT();
  const { locale } = useLocale();
  const [status, setStatus] = useState<Status>("open");
  const issues = useIssues(status);
  const resolve = useResolve();
  const canResolve = useCan("ext_issue_watch_issue", "update");
  const when = new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" });
  const failure = issues.error ?? resolve.error;
  return (
    <div className="wrap">
      <SectionHeader title={t("extIssueWatch.page.title")} level={1} />
      <p>{t("extIssueWatch.page.sub")}</p>
      <SegmentedControl
        label={t("extIssueWatch.page.status")}
        options={statuses}
        value={status}
        onChange={setStatus}
        labels={{ open: t("extIssueWatch.status.open"), resolved: t("extIssueWatch.status.resolved"), all: t("extIssueWatch.status.all") }}
      />
      {failure && (
        <Callout tone="danger" title={t("extIssueWatch.page.failed")}>
          {statusOf(failure) === 403 ? t("extIssueWatch.needsGrant") : problemMessageOf(failure, t)}
        </Callout>
      )}
      {issues.data?.length === 0 && <EmptyState title={t("extIssueWatch.page.empty")}>{t("extIssueWatch.page.emptyBody")}</EmptyState>}
      {issues.data && issues.data.length > 0 && (
        <Card>
          <DataTable
            label={t("extIssueWatch.page.title")}
            rows={issues.data}
            rowKey={(issue) => issue.id}
            columns={[
              {
                key: "kind",
                header: t("extIssueWatch.col.kind"),
                render: (issue) => <Badge tone={kindTone[issue.kind]}>{t(`extIssueWatch.kind.${issue.kind}`)}</Badge>,
              },
              {
                key: "what",
                header: t("extIssueWatch.col.what"),
                grow: true,
                render: (issue) => (
                  <div className="ext-issue-watch-cell">
                    <code>{issue.title}</code>
                    {issue.detail && <span className="t-meta">{issue.detail}</span>}
                    {issue.reference && <Reference value={issue.reference} />}
                    <a className="t-meta" href={`#${issue.last_page}`}>
                      {issue.pattern}
                    </a>
                  </div>
                ),
              },
              { key: "times", header: t("extIssueWatch.col.times"), align: "end", render: (issue) => String(issue.occurrences) },
              {
                key: "seen",
                header: t("extIssueWatch.col.seen"),
                render: (issue) => (
                  <span className="t-meta">
                    {when.format(new Date(issue.last_seen))}
                    <br />
                    {t("extIssueWatch.firstSeen", { when: when.format(new Date(issue.first_seen)) })}
                  </span>
                ),
              },
              {
                key: "resolve",
                header: t("extIssueWatch.col.resolve"),
                headerHidden: true,
                render: (issue) =>
                  issue.resolved_at ? (
                    <Badge tone="success">{t("extIssueWatch.status.resolved")}</Badge>
                  ) : canResolve ? (
                    <Button disabled={resolve.isPending} onClick={() => resolve.mutate(issue.id)}>
                      {t("extIssueWatch.resolve")}
                    </Button>
                  ) : null,
              },
            ]}
          />
        </Card>
      )}
      <Settings />
    </div>
  );
}

// What counts as an issue: everyone sees it, only an admin changes it.
function Settings() {
  const t = useT();
  const config = useConfig();
  const save = useSaveConfig();
  const canEdit = useCan("ext_issue_watch_config", "update");
  const [draft, setDraft] = useState<Config | null>(null);
  useEffect(() => {
    if (config.data) setDraft(config.data);
  }, [config.data]);
  if (!draft) return null;
  const set = <K extends keyof Config>(key: K, value: Config[K]) => setDraft({ ...draft, [key]: value });
  const seconds = (ms: number) => String(ms / 1000);
  const toMs = (text: string) => Math.max(0, Math.round(Number(text.replace(",", ".")) * 1000) || 0);
  const switches: { key: "server_errors" | "refusals" | "app_errors" | "ui_problems" | "toasts"; label: string }[] = [
    { key: "server_errors", label: t("extIssueWatch.set.server") },
    { key: "refusals", label: t("extIssueWatch.set.refusals") },
    { key: "app_errors", label: t("extIssueWatch.set.app") },
    { key: "ui_problems", label: t("extIssueWatch.set.ui") },
    { key: "toasts", label: t("extIssueWatch.set.toasts") },
  ];
  return (
    <Card>
      <SectionHeader title={t("extIssueWatch.set.title")} />
      <p className="t-meta">{t(canEdit ? "extIssueWatch.set.sub" : "extIssueWatch.set.readOnly")}</p>
      <div className="form-stack">
        {switches.map(({ key, label }) => (
          <label key={key} className="ext-issue-watch-switch">
            <input type="checkbox" checked={draft[key]} disabled={!canEdit} onChange={(event) => set(key, event.target.checked)} />
            {label}
          </label>
        ))}
        <Field label={t("extIssueWatch.set.slow")}>
          {(control) => (
            <TextInput
              {...control}
              inputMode="decimal"
              disabled={!canEdit}
              value={seconds(draft.slow_ms)}
              onChange={(event) => set("slow_ms", toMs(event.target.value))}
            />
          )}
        </Field>
        <Field label={t("extIssueWatch.set.stuck")}>
          {(control) => (
            <TextInput
              {...control}
              inputMode="decimal"
              disabled={!canEdit}
              value={seconds(draft.stuck_ms)}
              onChange={(event) => set("stuck_ms", toMs(event.target.value))}
            />
          )}
        </Field>
        <Field label={t("extIssueWatch.set.ignore")}>
          {(control) => (
            <textarea
              {...control}
              className="input"
              rows={4}
              disabled={!canEdit}
              value={draft.ignore.join("\n")}
              onChange={(event) => set("ignore", event.target.value.split("\n"))}
            />
          )}
        </Field>
        {save.error && <p className="t-meta ext-issue-watch-error">{problemMessageOf(save.error, t)}</p>}
        {canEdit && (
          <Row gap="2" justify="end">
            {save.isSuccess && <span className="t-meta">{t("extIssueWatch.set.saved")}</span>}
            <Button
              variant="primary"
              disabled={save.isPending}
              onClick={() => save.mutate({ ...draft, ignore: draft.ignore.map((line) => line.trim()).filter(Boolean) })}
            >
              {t("extIssueWatch.set.save")}
            </Button>
          </Row>
        )}
      </div>
    </Card>
  );
}
