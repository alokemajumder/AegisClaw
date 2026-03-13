"use client";

import { use } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ArrowLeft, Loader2, Play, CheckCircle2, XCircle, Clock, Pause } from "lucide-react";
import Link from "next/link";
import { usePolling } from "@/hooks/useApi";
import { getRun, listRunSteps, killRun } from "@/lib/api";
import type { Run, RunStep } from "@/lib/types";
import { useState } from "react";

const statusIcon: Record<string, React.ReactNode> = {
  pending: <Clock className="h-4 w-4 text-slate-400" />,
  running: <Play className="h-4 w-4 text-blue-500" />,
  completed: <CheckCircle2 className="h-4 w-4 text-emerald-500" />,
  failed: <XCircle className="h-4 w-4 text-red-500" />,
  skipped: <Pause className="h-4 w-4 text-slate-400" />,
};

const statusColor: Record<string, string> = {
  queued: "bg-slate-100 text-slate-700",
  running: "bg-blue-100 text-blue-700",
  completed: "bg-emerald-100 text-emerald-700",
  failed: "bg-red-100 text-red-700",
  killed: "bg-red-100 text-red-700",
  paused: "bg-amber-100 text-amber-700",
};

export default function RunDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: run, loading, error } = usePolling<Run>(() => getRun(id).then(r => ({ data: r.data })), 5000, [id]);
  const { data: steps, refetch: refetchSteps } = usePolling<RunStep[]>(() => listRunSteps(id).then(r => ({ data: r.data })), 5000, [id]);
  const [killing, setKilling] = useState(false);

  const handleKill = async () => {
    setKilling(true);
    try {
      await killRun(id);
      refetchSteps();
    } catch { /* handled */ } finally {
      setKilling(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-slate-500 py-12 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading run...
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
        Failed to load data: {error}
      </div>
    );
  }

  if (!run) {
    return <div className="text-center py-12 text-slate-400">Run not found</div>;
  }

  const progress = run.steps_total > 0 ? Math.round((run.steps_completed / run.steps_total) * 100) : 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/runs">
            <Button variant="ghost" size="sm" aria-label="Back to runs"><ArrowLeft className="h-4 w-4" /></Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold text-slate-900">Run {run.id.slice(0, 8)}</h1>
            <p className="text-sm text-slate-500">Engagement {run.engagement_id.slice(0, 8)}</p>
          </div>
        </div>
        {(run.status === "running" || run.status === "queued") && (
          <Button variant="destructive" size="sm" onClick={handleKill} disabled={killing}>
            {killing ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <XCircle className="h-4 w-4 mr-1" />}
            Kill Run
          </Button>
        )}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Status</CardTitle></CardHeader>
          <CardContent>
            <Badge className={statusColor[run.status] ?? "bg-slate-100 text-slate-700"}>{run.status}</Badge>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Tier</CardTitle></CardHeader>
          <CardContent><span className="text-lg font-semibold">{run.tier}</span></CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Progress</CardTitle></CardHeader>
          <CardContent>
            <div className="flex items-center gap-2">
              <div className="flex-1 bg-slate-100 rounded-full h-2">
                <div className="bg-emerald-500 h-2 rounded-full transition-all" style={{ width: `${progress}%` }} />
              </div>
              <span className="text-sm font-medium">{progress}%</span>
            </div>
            <p className="text-xs text-slate-400 mt-1">{run.steps_completed}/{run.steps_total} steps</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Duration</CardTitle></CardHeader>
          <CardContent>
            <span className="text-sm">
              {run.started_at ? new Date(run.started_at).toLocaleTimeString() : "—"}
              {run.completed_at ? ` → ${new Date(run.completed_at).toLocaleTimeString()}` : ""}
            </span>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Steps</CardTitle>
        </CardHeader>
        <CardContent>
          {steps && steps.length > 0 ? (
            <div className="space-y-3">
              {steps.sort((a, b) => a.step_number - b.step_number).map((step) => (
                <div key={step.id} className="flex items-start gap-3 p-3 rounded-lg border border-slate-100">
                  <div className="mt-0.5">{statusIcon[step.status] ?? statusIcon.pending}</div>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-slate-900">Step {step.step_number}</span>
                      <Badge variant="outline" className="text-xs">{step.agent_type}</Badge>
                      {step.technique_id && <Badge variant="outline" className="text-xs">{step.technique_id}</Badge>}
                    </div>
                    <p className="text-xs text-slate-500 mt-0.5">{step.status}</p>
                    {step.error_message && (
                      <p className="text-xs text-red-500 mt-1">{step.error_message}</p>
                    )}
                  </div>
                  <div className="text-xs text-slate-400">
                    {step.completed_at ? new Date(step.completed_at).toLocaleTimeString() : ""}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-center text-slate-400 py-6">No steps yet</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
