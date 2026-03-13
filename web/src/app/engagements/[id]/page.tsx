"use client";

import { use, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { ArrowLeft, Loader2, Play, Calendar } from "lucide-react";
import Link from "next/link";
import { useApi } from "@/hooks/useApi";
import { getEngagement, listEngagementRuns, triggerRun } from "@/lib/api";
import type { Engagement, Run } from "@/lib/types";

const statusColor: Record<string, string> = {
  draft: "bg-slate-100 text-slate-700",
  active: "bg-emerald-100 text-emerald-700",
  paused: "bg-amber-100 text-amber-700",
  completed: "bg-blue-100 text-blue-700",
  archived: "bg-slate-100 text-slate-700",
};

const runStatusColor: Record<string, string> = {
  queued: "bg-slate-100 text-slate-700",
  running: "bg-blue-100 text-blue-700",
  completed: "bg-emerald-100 text-emerald-700",
  failed: "bg-red-100 text-red-700",
  killed: "bg-red-100 text-red-700",
  paused: "bg-amber-100 text-amber-700",
};

export default function EngagementDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: engagement, loading, error } = useApi<Engagement>(() => getEngagement(id).then(r => ({ data: r.data })), [id]);
  const { data: runs, refetch: refetchRuns } = useApi<Run[]>(() => listEngagementRuns(id).then(r => ({ data: r.data })), [id]);
  const [triggering, setTriggering] = useState(false);

  const handleTrigger = async () => {
    setTriggering(true);
    try {
      await triggerRun(id);
      refetchRuns();
    } catch { /* handled */ } finally {
      setTriggering(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-slate-500 py-12 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading engagement...
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

  if (!engagement) {
    return <div className="text-center py-12 text-slate-400">Engagement not found</div>;
  }

  const completedRuns = runs?.filter(r => r.status === "completed").length ?? 0;
  const failedRuns = runs?.filter(r => r.status === "failed").length ?? 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/engagements">
            <Button variant="ghost" size="sm" aria-label="Back to engagements"><ArrowLeft className="h-4 w-4" /></Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold text-slate-900">{engagement.name}</h1>
            <p className="text-sm text-slate-500">{engagement.description || engagement.id.slice(0, 8)}</p>
          </div>
        </div>
        {engagement.status === "active" && (
          <Button onClick={handleTrigger} disabled={triggering}>
            {triggering ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <Play className="h-4 w-4 mr-1" />}
            Trigger Run
          </Button>
        )}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Status</CardTitle></CardHeader>
          <CardContent>
            <Badge className={statusColor[engagement.status]}>{engagement.status}</Badge>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Allowed Tiers</CardTitle></CardHeader>
          <CardContent>
            <div className="flex gap-1">
              {engagement.allowed_tiers.map(t => (
                <Badge key={t} variant="outline">Tier {t}</Badge>
              ))}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Total Runs</CardTitle></CardHeader>
          <CardContent><span className="text-lg font-semibold">{runs?.length ?? 0}</span></CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Success Rate</CardTitle></CardHeader>
          <CardContent>
            <span className="text-lg font-semibold">
              {runs && runs.length > 0 ? Math.round((completedRuns / (completedRuns + failedRuns || 1)) * 100) : 0}%
            </span>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Configuration</CardTitle></CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-slate-500">Rate Limit</span>
            <span>{engagement.rate_limit} req/min</span>
          </div>
          <div className="flex justify-between">
            <span className="text-slate-500">Concurrency Cap</span>
            <span>{engagement.concurrency_cap}</span>
          </div>
          {engagement.schedule_cron && (
            <div className="flex justify-between">
              <span className="text-slate-500">Schedule</span>
              <div className="flex items-center gap-1">
                <Calendar className="h-3 w-3 text-slate-400" />
                <span className="font-mono text-xs">{engagement.schedule_cron}</span>
              </div>
            </div>
          )}
          <div className="flex justify-between">
            <span className="text-slate-500">Created</span>
            <span>{new Date(engagement.created_at).toLocaleDateString()}</span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Run History</CardTitle>
        </CardHeader>
        <CardContent>
          {runs && runs.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Run ID</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Tier</TableHead>
                  <TableHead>Progress</TableHead>
                  <TableHead>Date</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map(run => (
                  <TableRow key={run.id}>
                    <TableCell>
                      <Link href={`/runs/${run.id}`} className="text-blue-600 hover:underline font-mono text-xs">
                        {run.id.slice(0, 8)}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <Badge className={runStatusColor[run.status] ?? "bg-slate-100 text-slate-700"}>{run.status}</Badge>
                    </TableCell>
                    <TableCell>{run.tier}</TableCell>
                    <TableCell>{run.steps_completed}/{run.steps_total}</TableCell>
                    <TableCell className="text-slate-500 text-xs">{new Date(run.created_at).toLocaleDateString()}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <p className="text-center text-slate-400 py-6">No runs yet. Click &quot;Trigger Run&quot; to start one.</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
