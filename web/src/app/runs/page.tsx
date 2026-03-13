"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Activity, Loader2 } from "lucide-react";
import { usePolling } from "@/hooks/useApi";
import { listRuns } from "@/lib/api";
import type { Run } from "@/lib/types";

const statusColor: Record<string, string> = {
  running: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  queued: "bg-slate-100 text-slate-700 hover:bg-slate-100",
  completed: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  failed: "bg-red-100 text-red-700 hover:bg-red-100",
  killed: "bg-red-100 text-red-700 hover:bg-red-100",
  paused: "bg-amber-100 text-amber-700 hover:bg-amber-100",
};

const tierColor: Record<number, string> = {
  0: "bg-slate-100 text-slate-700 hover:bg-slate-100",
  1: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  2: "bg-indigo-100 text-indigo-700 hover:bg-indigo-100",
  3: "bg-red-100 text-red-700 hover:bg-red-100",
};

export default function RunsPage() {
  const [filter, setFilter] = useState("all");

  const { data: runs, loading, error } = usePolling<Run[]>(
    () => listRuns(1, 50, filter === "all" ? undefined : filter),
    5000,
    [filter]
  );

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Runs</h1>
        <p className="text-sm text-slate-500">
          Track and monitor security validation runs (auto-refreshes every 5s)
        </p>
      </div>

      <Tabs defaultValue="all" onValueChange={setFilter}>
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="running">Running</TabsTrigger>
          <TabsTrigger value="completed">Completed</TabsTrigger>
          <TabsTrigger value="failed">Failed</TabsTrigger>
        </TabsList>
      </Tabs>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Activity className="h-4 w-4" />
            Run History
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loading && !runs ? (
            <div className="flex items-center gap-2 text-slate-500 py-8 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading runs...
            </div>
          ) : error ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
              Failed to load data: {error}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Run ID</TableHead>
                  <TableHead>Tier</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Steps</TableHead>
                  <TableHead>Started</TableHead>
                  <TableHead>Completed</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs && runs.length > 0 ? (
                  runs.map((run) => (
                    <TableRow key={run.id}>
                      <TableCell className="font-mono text-sm font-medium text-slate-900">
                        {run.id.slice(0, 8)}
                      </TableCell>
                      <TableCell>
                        <Badge className={tierColor[run.tier] ?? "bg-slate-100 text-slate-700"}>
                          Tier {run.tier}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge className={statusColor[run.status] ?? "bg-slate-100 text-slate-700"}>
                          {run.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-slate-600">
                        {run.steps_completed}/{run.steps_total}
                      </TableCell>
                      <TableCell className="text-slate-600">
                        {run.started_at
                          ? new Date(run.started_at).toLocaleString()
                          : "—"}
                      </TableCell>
                      <TableCell className="text-slate-600">
                        {run.completed_at
                          ? new Date(run.completed_at).toLocaleString()
                          : "—"}
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-slate-400 py-8">
                      No runs found
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
