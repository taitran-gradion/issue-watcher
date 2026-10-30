import { LocaleProvider } from "@margince/frontend/app";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRoot } from "react-dom/client";
import { startMonitor } from "./monitor";
import { IssuesPage } from "./page";
import { Watcher } from "./watcher";
import "./issue-watch.css";

// Core imports every extension's entry when the app starts, and gives a unit
// no place on every page. So the watcher starts here, before the app's first
// request, and mounts itself beside the app, once, with its own query client
// and locale.
const hostId = "ext-issue-watch-host";
if (typeof document !== "undefined" && window.self === window.top && !document.getElementById(hostId)) {
  startMonitor();
  const host = document.createElement("div");
  host.id = hostId;
  document.body.append(host);
  createRoot(host).render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider>
        <Watcher />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// #/ext/issue-watch: every issue, and what counts as one.
export default IssuesPage;
