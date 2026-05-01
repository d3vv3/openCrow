"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import dynamic from "next/dynamic";
import type { MemoryGraph, MemoryEntity, MemoryRelation } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { TrashIcon } from "@/components/ui/icons";

// react-force-graph-2d uses browser APIs, must be loaded client-side only
const ForceGraph2D = dynamic(() => import("react-force-graph-2d"), { ssr: false });

type NodeObject = {
  id: string;
  name: string;
  type: string;
  summary: string;
  val?: number;
  color?: string;
  x?: number;
  y?: number;
};

type LinkObject = {
  id: string;
  source: string | NodeObject;
  target: string | NodeObject;
  relation: string;
  confidence: number;
};

const TYPE_COLORS: Record<string, string> = {
  person: "#a78bfa",
  organization: "#38bdf8",
  place: "#34d399",
  project: "#f59e0b",
  trip: "#fb923c",
  event: "#f87171",
  topic: "#c084fc",
  preference: "#e879f9",
  language: "#60a5fa",
  food: "#4ade80",
  phone_number: "#94a3b8",
  email: "#cbd5e1",
  thing: "#475569",
};

// Extra palette for unknown/custom types
const EXTRA_PALETTE = [
  "#e879f9",
  "#2dd4bf",
  "#facc15",
  "#f472b6",
  "#818cf8",
  "#4ade80",
  "#fb7185",
  "#a3e635",
  "#22d3ee",
  "#fbbf24",
];
const dynamicColorCache: Record<string, string> = {};
let paletteIndex = 0;

function getColor(type: string): string {
  const key = type.toLowerCase();
  if (TYPE_COLORS[key]) return TYPE_COLORS[key];
  if (!dynamicColorCache[key]) {
    dynamicColorCache[key] = EXTRA_PALETTE[paletteIndex % EXTRA_PALETTE.length];
    paletteIndex++;
  }
  return dynamicColorCache[key];
}

// Refresh icon: two circular arrows with clear gap
function RefreshIcon({ spinning }: { spinning: boolean }) {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={spinning ? "animate-spin" : ""}
    >
      {/* Top-right arc with arrowhead */}
      <path d="M21 12a9 9 0 0 0-9-9 9 9 0 0 0-6.36 2.64L3 8" />
      <polyline points="3 3 3 8 8 8" />
      {/* Bottom-left arc with arrowhead */}
      <path d="M3 12a9 9 0 0 0 9 9 9 9 0 0 0 6.36-2.64L21 16" />
      <polyline points="21 21 21 16 16 16" />
    </svg>
  );
}

export function MemoryGraphTab({ onError }: { onError?: (msg: string) => void }) {
  const [graph, setGraph] = useState<MemoryGraph | null>(null);
  const [loading, setLoading] = useState(true);
  const [graphKey, setGraphKey] = useState(0); // bump to force remount after errors
  const [selected, setSelected] = useState<MemoryEntity | null>(null);
  const [selectedRelations, setSelectedRelations] = useState<MemoryRelation[]>([]);
  const [activeFilters, setActiveFilters] = useState<Set<string>>(new Set());
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const graphRef = useRef<any>(null); // ForceGraphMethods API includes runtime-only renderer()
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const g = await endpoints.getMemoryGraph();
      setGraph(g);
      setGraphKey((k) => k + 1);
    } catch {
      onError?.("Failed to load memory graph");
    } finally {
      setLoading(false);
    }
  }, [onError]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const obs = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        setDimensions({
          width: entry.contentRect.width,
          height: entry.contentRect.height,
        });
      }
    });
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  const handleDeleteEntity = async (id: string) => {
    try {
      await endpoints.deleteMemoryEntity(id);
      setSelected(null);
      setSelectedRelations([]);
      await load();
    } catch {
      onError?.("Failed to delete entity");
    }
  };

  const handleDeleteRelation = async (id: string) => {
    try {
      await endpoints.deleteMemoryRelation(id);
      setSelectedRelations((prev) => prev.filter((r) => r.id !== id));
      await load();
    } catch {
      onError?.("Failed to delete relation");
    }
  };

  const toggleFilter = (type: string) => {
    setActiveFilters((prev) => {
      const next = new Set(prev);
      if (next.has(type)) {
        next.delete(type);
      } else {
        next.add(type);
      }
      return next;
    });
  };

  const clearFilters = () => setActiveFilters(new Set());

  // Compute node degrees for centrality-based sizing
  const nodeDegrees = new Map<string, number>();
  for (const r of graph?.relations ?? []) {
    nodeDegrees.set(r.from_entity_id, (nodeDegrees.get(r.from_entity_id) ?? 0) + 1);
    nodeDegrees.set(r.to_entity_id, (nodeDegrees.get(r.to_entity_id) ?? 0) + 1);
  }

  const allNodes: NodeObject[] = (graph?.entities ?? []).map((e) => {
    const degree = nodeDegrees.get(e.id) ?? 0;
    return {
      id: e.id,
      name: e.name,
      type: e.type,
      summary: e.summary ?? "",
      val: Math.max(2, Math.min(12, 2 + degree * 1.2)),
      color: getColor(e.type),
    };
  });

  const presentTypes = Array.from(
    new Set([...Object.keys(TYPE_COLORS), ...allNodes.map((n) => n.type.toLowerCase())]),
  ).sort();

  // Filter nodes: if no filters active, show all
  const isFiltered = activeFilters.size > 0;
  const nodes = isFiltered
    ? allNodes.filter((n) => activeFilters.has(n.type.toLowerCase()))
    : allNodes;

  const visibleNodeIds = new Set(nodes.map((n) => n.id));

  const allLinks: LinkObject[] = (graph?.relations ?? []).map((r) => ({
    id: r.id,
    source: r.from_entity_id,
    target: r.to_entity_id,
    relation: r.relation,
    confidence: r.confidence,
  }));

  // Only show links where both endpoints are visible
  const links = isFiltered
    ? allLinks.filter(
        (l) => visibleNodeIds.has(l.source as string) && visibleNodeIds.has(l.target as string),
      )
    : allLinks;

  // Fix canvas resolution for high-DPI displays (runs after graph renders)
  useEffect(() => {
    const timer = setTimeout(() => {
      const instance = graphRef.current;
      if (instance) {
        try {
          const canvas = instance.renderer().domElement;
          const dpr = window.devicePixelRatio || 1;
          if (canvas.width !== dimensions.width * dpr) {
            canvas.width = dimensions.width * dpr;
            canvas.height = dimensions.height * dpr;
            canvas.style.width = dimensions.width + "px";
            canvas.style.height = dimensions.height + "px";
            instance.renderer().setPixelRatio(dpr);
          }
        } catch {
          /* canvas not ready yet */
        }
      }
    }, 50);
    return () => clearTimeout(timer);
  }, [dimensions, nodes]);

  // Tune force simulation for spacing (avoid label overlap)
  useEffect(() => {
    const instance = graphRef.current;
    if (!instance) return;
    const count = nodes.length;
    // More nodes = more repulsion and longer links
    instance.d3Force("charge")?.strength(-60 - count * 5);
    instance.d3Force("link")?.distance(50 + count * 3);
    // Reheat to recompute layout
    instance.d3ReheatSimulation();
  }, [nodes]);

  const handleNodeClick = (node: NodeObject) => {
    const entity = graph?.entities.find((e) => e.id === node.id);
    if (!entity) return;
    setSelected(entity);
    const rels = (graph?.relations ?? []).filter(
      (r) => r.from_entity_id === entity.id || r.to_entity_id === entity.id,
    );
    setSelectedRelations(rels);
  };

  const isEmpty = !loading && (!graph?.entities || graph.entities.length === 0);

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <header className="shrink-0 flex items-center justify-between px-6 py-4 border-b border-outline-ghost">
        <h1 className="font-display text-3xl font-semibold text-on-surface">Memory Graph</h1>
      </header>

      {/* Filter bar */}
      <div className="shrink-0 flex flex-wrap items-center gap-2 px-6 py-3 border-b border-outline-ghost">
        {/* "All" pill */}
        <button
          onClick={clearFilters}
          className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium transition-all cursor-pointer ${
            !isFiltered
              ? "bg-violet/20 text-violet border border-violet/40"
              : "text-on-surface-variant border border-outline-ghost hover:border-violet/30 hover:text-on-surface"
          }`}
        >
          All
        </button>

        {presentTypes.map((type) => {
          const active = activeFilters.has(type);
          const color = getColor(type);
          return (
            <button
              key={type}
              onClick={() => toggleFilter(type)}
              className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium transition-all cursor-pointer border ${
                active
                  ? "border-transparent text-white"
                  : "border-outline-ghost text-on-surface-variant hover:text-on-surface hover:border-white/20"
              }`}
              style={active ? { background: color, borderColor: color } : {}}
            >
              <span
                className="w-2 h-2 rounded-full shrink-0"
                style={{ background: color, opacity: active ? 1 : 0.7 }}
              />
              {type}
            </button>
          );
        })}

        {graph && (
          <span className="ml-auto text-xs text-on-surface-variant opacity-60">
            {isFiltered
              ? `${nodes.length} / ${graph.entities?.length ?? 0} entities`
              : `${graph.entities?.length ?? 0} entities . ${graph.relations?.length ?? 0} relations`}
          </span>
        )}
      </div>

      {/* Graph area */}
      <div className="flex flex-1 min-h-0 relative">
        {/* Canvas container */}
        <div ref={containerRef} className="flex-1 min-w-0 relative overflow-hidden">
          {isEmpty ? (
            <div className="flex items-center justify-center h-full text-on-surface-variant text-sm">
              No memory graph yet. Chat with the assistant to build it automatically.
            </div>
          ) : (
            <ForceGraph2D
              key={graphKey}
              ref={graphRef}
              graphData={{ nodes, links }}
              width={dimensions.width}
              height={dimensions.height}
              nodeLabel={(n) => {
                const node = n as NodeObject;
                return `${node.name} (${node.type})${node.summary ? "\n" + node.summary : ""}`;
              }}
              nodeColor={(n) => (n as NodeObject).color ?? "#94a3b8"}
              nodeVal={(n) => (n as NodeObject).val ?? 4}
              linkLabel={(l) => {
                const link = l as LinkObject;
                return `${link.relation} (${Math.round(link.confidence * 100)}%)`;
              }}
              linkWidth={(l) => Math.max(0.5, (l as LinkObject).confidence * 3)}
              linkColor={() => "rgba(148,163,184,0.4)"}
              linkDirectionalArrowLength={4}
              linkDirectionalArrowRelPos={1}
              onNodeClick={(n) => handleNodeClick(n as NodeObject)}
              backgroundColor="transparent"
              nodeCanvasObjectMode={() => "after"}
              nodeCanvasObject={(node, ctx, globalScale) => {
                const n = node as NodeObject;
                const maxLen = 20;
                const label = n.name.length > maxLen ? n.name.slice(0, maxLen - 1) + "..." : n.name;
                const fontSize = Math.max(8, Math.min(13, 11 / globalScale));
                ctx.font = `600 ${fontSize}px sans-serif`;
                ctx.textAlign = "center";
                ctx.textBaseline = "top";
                const textX = n.x ?? 0;
                const textY = (n.y ?? 0) + 8;
                const isDark =
                  typeof window !== "undefined" &&
                  window.matchMedia("(prefers-color-scheme: dark)").matches;
                ctx.fillStyle = isDark ? "#ffffff" : "#1e293b";
                ctx.fillText(label, textX, textY);
              }}
            />
          )}
        </div>

        {/* Floating refresh button */}
        <button
          onClick={load}
          disabled={loading}
          title="Refresh"
          className="absolute top-4 right-4 z-30 flex items-center justify-center w-10 h-10 rounded-full bg-surface-lowest/90 border border-violet/30 backdrop-blur-sm shadow-lg text-on-surface-variant hover:text-violet hover:border-violet/60 transition-all disabled:opacity-50 cursor-pointer hover:cursor-pointer"
        >
          <RefreshIcon spinning={loading} />
        </button>

        {/* Side panel -- glassy floating card */}
        {selected && (
          <div className="absolute right-4 top-16 max-h-[calc(100%-5rem)] w-72 z-20 rounded-2xl border border-white/10 bg-surface-lowest/80 backdrop-blur-2xl shadow-[var(--shadow-float)] overflow-hidden flex flex-col animate-fade-in">
            {/* Header */}
            <div className="shrink-0 px-4 pt-4 pb-3 flex items-start justify-between gap-2">
              <div className="min-w-0">
                <p className="font-semibold text-on-surface text-sm truncate">{selected.name}</p>
                <div className="flex items-center gap-1.5 mt-1">
                  <span
                    className="w-2 h-2 rounded-full shrink-0"
                    style={{ background: getColor(selected.type) }}
                  />
                  <p className="text-xs text-on-surface-variant capitalize">{selected.type}</p>
                </div>
              </div>
              <button
                onClick={() => setSelected(null)}
                className="text-on-surface-variant/70 hover:text-on-surface text-sm leading-none mt-0.5 cursor-pointer shrink-0"
                title="Close"
              >
                ✕
              </button>
            </div>

            {/* Summary */}
            {selected.summary && (
              <div className="px-4 pb-3">
                <p className="text-xs text-on-surface-variant leading-relaxed">
                  {selected.summary}
                </p>
              </div>
            )}

            {/* Divider */}
            <div className="border-t border-white/5" />

            {/* Relations */}
            <div className="max-h-52 overflow-y-auto px-4 py-3 space-y-1">
              <p className="text-[10px] font-medium text-on-surface-variant uppercase tracking-wider mb-2">
                Relations ({selectedRelations.length})
              </p>
              {selectedRelations.length === 0 && (
                <p className="text-xs text-on-surface-variant">No relations</p>
              )}
              {selectedRelations.map((r) => {
                const isFrom = r.from_entity_id === selected.id;
                const other = isFrom ? r.to_entity_name : r.from_entity_name;
                return (
                  <div
                    key={r.id}
                    className="group flex items-center justify-between gap-2 py-2 border-b border-white/5 last:border-0"
                  >
                    <div className="min-w-0">
                      <p className="text-xs text-on-surface truncate">
                        {isFrom ? "-> " : "<- "}
                        {other}
                      </p>
                      <p className="text-xs text-on-surface-variant italic truncate">
                        {r.relation}
                      </p>
                      <p className="text-[10px] text-on-surface-variant">
                        {Math.round(r.confidence * 100)}% . *{r.reinforcement_count}
                      </p>
                    </div>
                    <button
                      onClick={() => handleDeleteRelation(r.id)}
                      className="opacity-0 group-hover:opacity-100 text-error/60 hover:text-error shrink-0 cursor-pointer p-1 -m-1 transition-opacity"
                      title="Delete relation"
                    >
                      <TrashIcon width="16" height="16" />
                    </button>
                  </div>
                );
              })}
            </div>

            {/* Delete entity */}
            <div className="shrink-0 px-4 py-3">
              <Button
                variant="danger"
                size="sm"
                onClick={() => handleDeleteEntity(selected.id)}
                className="w-full min-h-[36px]"
              >
                Delete entity
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
