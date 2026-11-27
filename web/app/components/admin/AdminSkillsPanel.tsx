"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2, PackagePlus, ShieldCheck, Trash2 } from "lucide-react";
import {
  agentApi,
  type Skill,
  type SkillVersion,
  type Visibility,
} from "@/lib/api/agents";
import { useIsPlatformAdmin } from "@/hooks/useIsPlatformAdmin";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : "Request failed";
}

export function AdminSkillsPanel() {
  const queryClient = useQueryClient();
  const canManage = useIsPlatformAdmin();
  const [skillSlug, setSkillSlug] = useState("");
  const [skillDisplay, setSkillDisplay] = useState("");
  const [skillVisibility, setSkillVisibility] =
    useState<Visibility>("private");
  const [skillContent, setSkillContent] = useState("");
  const [selected, setSelected] = useState<Skill | null>(null);
  const [versions, setVersions] = useState<SkillVersion[]>([]);
  const [newContent, setNewContent] = useState("");

  const skillsQuery = useQuery({
    queryKey: ["admin", "skills"],
    queryFn: () => agentApi.listSkills(),
    refetchInterval: 30_000,
  });

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["admin", "skills"] });
    void queryClient.invalidateQueries({ queryKey: ["agents", "catalog"] });
  };

  const create = useMutation({
    mutationFn: () =>
      agentApi.createSkill({
        slug: skillSlug.trim(),
        displayName: skillDisplay.trim() || undefined,
        visibility: skillVisibility,
        content: skillContent,
      }),
    onSuccess: () => {
      toast.success("Skill created");
      setSkillSlug("");
      setSkillDisplay("");
      setSkillContent("");
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const act = useMutation({
    mutationFn: async ({
      task,
      success,
    }: {
      task: () => Promise<unknown>;
      success: string;
    }) => {
      await task();
      return success;
    },
    onSuccess: (message) => {
      toast.success(message);
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const openSkill = async (skill: Skill) => {
    try {
      const detail = await agentApi.getSkill(skill.id);
      setSelected(detail);
      setVersions(detail.versions ?? []);
      setNewContent("");
    } catch (error) {
      toast.error(errorText(error));
    }
  };

  const addVersion = useMutation({
    mutationFn: async () => {
      if (!selected) throw new Error("No skill selected");
      return agentApi.addSkillVersion(selected.id, newContent);
    },
    onSuccess: async () => {
      toast.success("Skill version added");
      setNewContent("");
      if (selected) await openSkill(selected);
      refresh();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const skills = skillsQuery.data?.skills ?? [];

  return (
    <div className="space-y-5">
      <div className="rounded-xl border bg-card p-4">
        <div>
          <h2 className="text-sm font-semibold">Skills</h2>
          <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
            Skills add behavior instructions to an agent. They cannot grant
            tools. Shared and public skills require platform review.
          </p>
        </div>
        <div className="mt-4 space-y-3 rounded-lg border p-3">
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="block text-xs font-medium">
              Slug
              <Input
                className="mt-1"
                value={skillSlug}
                onChange={(event) => setSkillSlug(event.target.value)}
                placeholder="code-review"
              />
            </label>
            <label className="block text-xs font-medium">
              Display name
              <Input
                className="mt-1"
                value={skillDisplay}
                onChange={(event) => setSkillDisplay(event.target.value)}
              />
            </label>
            <label className="block text-xs font-medium">
              Visibility
              <select
                className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm"
                value={skillVisibility}
                onChange={(event) =>
                  setSkillVisibility(event.target.value as Visibility)
                }
              >
                <option value="private">private</option>
                <option value="shared">shared</option>
                <option value="public">public</option>
              </select>
            </label>
          </div>
          <label className="block text-xs font-medium">
            Content (SKILL.md)
            <textarea
              className="mt-1 min-h-32 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={skillContent}
              onChange={(event) => setSkillContent(event.target.value)}
              placeholder={
                "---\nname: code-review\ndescription: Review changed code\n---\n\nReview the changes…"
              }
            />
          </label>
          <div className="flex justify-end">
            <Button
              size="sm"
              disabled={
                create.isPending || !skillSlug.trim() || !skillContent.trim()
              }
              onClick={() => create.mutate()}
            >
              {create.isPending ? (
                <Loader2 className="animate-spin" />
              ) : (
                <PackagePlus />
              )}
              Create skill
            </Button>
          </div>
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="border-b px-4 py-3">
          <h2 className="text-sm font-semibold">Registered skills</h2>
        </div>
        {skills.length === 0 ? (
          <p className="p-6 text-center text-sm text-muted-foreground">
            No skills registered.
          </p>
        ) : (
          <div className="divide-y">
            {skills.map((skill) => (
              <div
                key={skill.id}
                className="flex flex-wrap items-center gap-2 px-4 py-3 text-sm"
              >
                <button
                  className="font-medium hover:underline"
                  onClick={() => void openSkill(skill)}
                >
                  {skill.displayName || skill.slug}
                </button>
                <Badge variant="secondary" className="text-[10px]">
                  v{skill.latestVersion} · {skill.visibility}
                </Badge>
                <Badge
                  variant="secondary"
                  className={cn(
                    "text-[10px]",
                    skill.reviewStatus === "approved" && "bg-success/10 text-success",
                    skill.reviewStatus === "pending" && "bg-warning/10 text-warning"
                  )}
                >
                  {skill.reviewStatus}
                </Badge>
                {canManage && skill.reviewStatus === "pending" && (
                  <>
                    <Button
                      size="xs"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () => agentApi.reviewSkill(skill.id, true),
                          success: "Skill approved",
                        })
                      }
                    >
                      <ShieldCheck />
                      Approve
                    </Button>
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () => agentApi.reviewSkill(skill.id, false),
                          success: "Skill rejected",
                        })
                      }
                    >
                      Reject
                    </Button>
                  </>
                )}
                <Button
                  size="xs"
                  variant="ghost"
                  className="ml-auto"
                  disabled={act.isPending}
                  onClick={() =>
                    act.mutate({
                      task: () => agentApi.deleteSkill(skill.id),
                      success: "Skill deleted",
                    })
                  }
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>

      {selected && (
        <div className="space-y-3 rounded-xl border bg-card p-4">
          <div className="flex items-center justify-between gap-2">
            <div>
              <h2 className="text-sm font-semibold">
                {selected.displayName || selected.slug}
              </h2>
              <p className="text-xs text-muted-foreground">
                {versions.length} version{versions.length === 1 ? "" : "s"}
              </p>
            </div>
            <Button size="xs" variant="ghost" onClick={() => setSelected(null)}>
              Close
            </Button>
          </div>
          <div className="divide-y rounded-lg border">
            {versions.map((version) => (
              <div
                key={version.id}
                className="flex flex-wrap items-center gap-2 px-3 py-2 text-xs"
              >
                <span className="font-medium">v{version.version}</span>
                <span className="text-muted-foreground">
                  {version.checksum.slice(0, 10)}…
                </span>
                <Badge variant="secondary" className="text-[10px]">
                  {version.reviewStatus}
                </Badge>
                {canManage && version.reviewStatus === "pending" && (
                  <>
                    <Button
                      size="xs"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () =>
                            agentApi.reviewSkillVersion(
                              selected.id,
                              version.version,
                              true
                            ),
                          success: "Version approved",
                        })
                      }
                    >
                      Approve
                    </Button>
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={act.isPending}
                      onClick={() =>
                        act.mutate({
                          task: () =>
                            agentApi.reviewSkillVersion(
                              selected.id,
                              version.version,
                              false
                            ),
                          success: "Version rejected",
                        })
                      }
                    >
                      Reject
                    </Button>
                  </>
                )}
                {version.content && (
                  <details className="w-full">
                    <summary className="cursor-pointer text-muted-foreground">
                      Preview content
                    </summary>
                    <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap text-[11px]">
                      {version.content}
                    </pre>
                  </details>
                )}
              </div>
            ))}
            {versions.length === 0 && (
              <p className="p-3 text-xs text-muted-foreground">
                No versions returned.
              </p>
            )}
          </div>
          <label className="block text-xs font-medium">
            New version content
            <textarea
              className="mt-1 min-h-28 w-full resize-y rounded-md border bg-background px-3 py-2 font-mono text-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
              value={newContent}
              onChange={(event) => setNewContent(event.target.value)}
            />
          </label>
          <div className="flex justify-end">
            <Button
              size="sm"
              disabled={addVersion.isPending || !newContent.trim()}
              onClick={() => addVersion.mutate()}
            >
              {addVersion.isPending && <Loader2 className="animate-spin" />}
              Add version
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
