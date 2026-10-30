import { api, ProblemError, throwProblem } from "@margince/frontend/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Config, defaultConfig } from "./monitor";

const configKey = ["ext", "issue-watch", "config"] as const;
const issuesKey = ["ext", "issue-watch", "issues"] as const;

export type Status = "open" | "resolved" | "all";

// What counts as an issue. A member whose role may not read it watches with the defaults.
export function useConfig() {
  return useQuery({
    queryKey: configKey,
    retry: false,
    queryFn: async (): Promise<Config> => {
      const { data, error, response } = await api.GET(
        "/ext/issue-watch/config",
      );
      if (response.status === 403) return defaultConfig;
      if (error) throwProblem(error);
      return data;
    },
  });
}

export function useSaveConfig() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (config: Config) => {
      const { data, error } = await api.POST("/ext/issue-watch/config", {
        body: config,
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: (config) => client.setQueryData(configKey, config),
  });
}

export function useIssues(status: Status) {
  return useQuery({
    queryKey: [...issuesKey, status],
    queryFn: async () => {
      const { data, error } = await api.GET("/ext/issue-watch/issues", {
        params: { query: { status } },
      });
      if (error) throwProblem(error);
      return data.issues;
    },
  });
}

// The open GitHub issues about the screen a page is on. The server refreshes
// its copy of the tracker every 15 minutes, so a member asks no more often.
export function useScreenGithub(page: string, enabled: boolean) {
  return useQuery({
    queryKey: ["ext", "issue-watch", "github", page],
    enabled,
    retry: false,
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ext/issue-watch/github", {
        params: { query: { page } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
}

export function useResolve() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { data, error } = await api.POST(
        "/ext/issue-watch/issues/resolve",
        { body: { id } },
      );
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: () => client.invalidateQueries({ queryKey: issuesKey }),
  });
}

export function statusOf(error: unknown): number | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  const problem = error.problem;
  return typeof problem === "object" &&
    problem !== null &&
    "status" in problem &&
    typeof problem.status === "number"
    ? problem.status
    : undefined;
}
