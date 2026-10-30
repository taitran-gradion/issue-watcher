import { useSyncExternalStore } from "react";

// The page the reader is on, as the app routes it: "#/contacts/abc?tab=x" is
// "/contacts/abc".
export function currentPage(): string {
  const route = window.location.hash.replace(/^#/, "").split("?")[0] ?? "";
  return route.startsWith("/") ? route.slice(0, 300) : "/";
}

// An id in a route: a UUID, or a number.
const idSegment =
  /^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\d+)$/i;

// The kind of page a route is: every contact's page is "/contacts/:id".
export function patternOf(page: string): string {
  return page
    .split("/")
    .map((segment) => (idSegment.test(segment) ? ":id" : segment))
    .join("/");
}

// The page the reader is on, kept current as the app moves between pages.
export function usePage(): string {
  return useSyncExternalStore((changed) => {
    window.addEventListener("hashchange", changed);
    return () => window.removeEventListener("hashchange", changed);
  }, currentPage);
}
