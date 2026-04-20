"use client";

import { useState } from "react";
import type { SkillEntry, SkillFile } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { TextArea } from "@/components/ui/TextArea";
import { Toggle } from "@/components/ui/Toggle";
import { SectionHeader } from "@/components/ui/SectionHeader";
import type { UpdateConfigFn } from "./types";
import { SkillFileCard } from "./SkillFileCard";

function CustomSkillCard({
  skill,
  index: i,
  updateConfig,
}: {
  skill: SkillEntry;
  index: number;
  updateConfig: UpdateConfigFn;
}) {
  const [expanded, setExpanded] = useState(!skill.name);

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-white/5 transition-colors"
        onClick={() => setExpanded((v) => !v)}
      >
        <span className="font-medium text-sm flex-1 truncate">
          {skill.name || `Skill ${i + 1}`}
        </span>
        {skill.description && (
          <span className="text-xs text-on-surface-variant truncate hidden sm:block max-w-[240px]">
            {skill.description}
          </span>
        )}
        <span
          className={`text-[10px] font-mono px-1.5 py-0.5 rounded ${skill.enabled ? "text-cyan bg-cyan/10" : "text-on-surface-variant bg-white/5"}`}
        >
          {skill.enabled ? "on" : "off"}
        </span>
        <svg
          className={`w-4 h-4 text-on-surface-variant transition-transform flex-shrink-0 ${expanded ? "rotate-180" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {expanded && (
        <div className="px-4 pb-4 pt-4 space-y-4 border-t border-white/10">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input
              label="Name"
              value={skill.name}
              onChange={(e) =>
                updateConfig((c) => {
                  c.skills.entries[i].name = e.target.value;
                  return c;
                })
              }
            />
            <Input
              label="Description"
              value={skill.description}
              onChange={(e) =>
                updateConfig((c) => {
                  c.skills.entries[i].description = e.target.value;
                  return c;
                })
              }
            />
          </div>
          <TextArea
            label="Content"
            value={skill.content}
            onChange={(e) =>
              updateConfig((c) => {
                c.skills.entries[i].content = e.target.value;
                return c;
              })
            }
            rows={6}
          />
          <div className="flex items-center gap-4">
            <Toggle
              label="Enabled"
              checked={skill.enabled}
              onChange={(v) =>
                updateConfig((c) => {
                  c.skills.entries[i].enabled = v;
                  return c;
                })
              }
            />
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto hover:text-error"
              onClick={() =>
                updateConfig((c) => {
                  c.skills.entries.splice(i, 1);
                  return c;
                })
              }
            >
              Remove
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

export function SkillsTab({
  initialSkillFiles,
  onSuccess,
  onError,
}: {
  initialSkillFiles: SkillFile[];
  onSuccess: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const [skillFiles, setSkillFiles] = useState<SkillFile[]>(initialSkillFiles);
  const [installSource, setInstallSource] = useState("");
  const [installing, setInstalling] = useState(false);
  const [installExpanded, setInstallExpanded] = useState(false);
  const [editingSkill, setEditingSkill] = useState<SkillFile | null>(null);

  return (
    <div className="space-y-4">
      <SectionHeader
        title="Installed Skills"
        description="SKILL.md files installed from GitHub or created manually"
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setEditingSkill({ slug: "", name: "", description: "", content: "" })}
          >
            New Skill
          </Button>
        }
      />

      <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
        <button
          className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-white/5 transition-colors"
          onClick={() => setInstallExpanded((v) => !v)}
        >
          <span className="font-medium text-sm flex-1">Install from GitHub</span>
          <svg
            className={`w-4 h-4 text-on-surface-variant transition-transform flex-shrink-0 ${installExpanded ? "rotate-180" : ""}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
          </svg>
        </button>
        {installExpanded && (
          <div className="px-4 pb-4 pt-4 space-y-3 border-t border-white/10">
            <Input
              label="GitHub source (owner/repo)"
              value={installSource}
              onChange={(e) => setInstallSource(e.target.value)}
              placeholder="e.g. myorg/my-skills"
            />
            <Button
              variant="secondary"
              size="sm"
              loading={installing}
              onClick={async () => {
                if (!installSource.trim()) return;
                setInstalling(true);
                try {
                  const result = await endpoints.installSkills(installSource.trim());
                  const updated = await endpoints.listSkillFiles();
                  setSkillFiles(updated);
                  setInstallSource("");
                  onSuccess(`Installed ${result.count} skill(s)`);
                  setInstallExpanded(false);
                } catch (e) {
                  onError(e instanceof Error ? e.message : "Install failed");
                } finally {
                  setInstalling(false);
                }
              }}
            >
              Install Skills
            </Button>
          </div>
        )}
      </div>

      {skillFiles.length === 0 && (
        <p className="text-on-surface-variant text-sm">No skill files installed.</p>
      )}
      {skillFiles.map((sf) => (
        <SkillFileCard
          key={sf.slug}
          sf={sf}
          onSave={async () => {
            const updated = await endpoints.listSkillFiles();
            setSkillFiles(updated);
          }}
          onDelete={(slug) => setSkillFiles((prev) => prev.filter((s) => s.slug !== slug))}
        />
      ))}

      {editingSkill !== null && (
        <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
          <div className="px-4 py-3 border-b border-white/10">
            <span className="font-medium text-sm">New Skill</span>
          </div>
          <div className="px-4 pb-4 pt-4 space-y-3">
            <Input
              label="Folder name (slug)"
              value={editingSkill.slug}
              onChange={(e) => setEditingSkill({ ...editingSkill, slug: e.target.value })}
              placeholder="my-skill"
            />
            <Input
              label="Display name"
              value={editingSkill.name}
              onChange={(e) => setEditingSkill({ ...editingSkill, name: e.target.value })}
              placeholder="My Skill"
            />
            <Input
              label="Description"
              value={editingSkill.description}
              onChange={(e) => setEditingSkill({ ...editingSkill, description: e.target.value })}
              placeholder="What this skill does"
            />
            <TextArea
              label="Content (SKILL.md)"
              value={editingSkill.content ?? ""}
              onChange={(e) => setEditingSkill({ ...editingSkill, content: e.target.value })}
              rows={12}
            />
            <div className="flex gap-2">
              <Button
                variant="primary"
                size="sm"
                onClick={async () => {
                  if (!editingSkill.slug) return;
                  await endpoints.createSkillFile({
                    name: editingSkill.slug,
                    description: editingSkill.description,
                    content: editingSkill.content ?? "",
                  });
                  const updated = await endpoints.listSkillFiles();
                  setSkillFiles(updated);
                  setEditingSkill(null);
                }}
              >
                Save
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setEditingSkill(null)}>
                Cancel
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export { CustomSkillCard };
