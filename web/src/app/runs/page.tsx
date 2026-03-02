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
import { Activity } from "lucide-react";

const runs = [
  {
    id: "RUN-347",
    engagement: "Quarterly AD Validation",
    tier: "Tier 0",
    status: "Completed",
    stepsCompleted: 15,
    stepsTotal: 15,
    started: "2024-01-15 09:30",
    duration: "12m 34s",
  },
  {
    id: "RUN-346",
    engagement: "Endpoint Detection Coverage",
    tier: "Tier 1",
    status: "Running",
    stepsCompleted: 8,
    stepsTotal: 14,
    started: "2024-01-15 09:45",
    duration: "6m 12s",
  },
  {
    id: "RUN-345",
    engagement: "Cloud Security Posture",
    tier: "Tier 2",
    status: "Completed",
    stepsCompleted: 10,
    stepsTotal: 10,
    started: "2024-01-15 08:00",
    duration: "8m 45s",
  },
  {
    id: "RUN-344",
    engagement: "Quarterly AD Validation",
    tier: "Tier 0",
    status: "Failed",
    stepsCompleted: 3,
    stepsTotal: 15,
    started: "2024-01-14 22:00",
    duration: "2m 10s",
  },
  {
    id: "RUN-343",
    engagement: "Endpoint Detection Coverage",
    tier: "Tier 1",
    status: "Completed",
    stepsCompleted: 14,
    stepsTotal: 14,
    started: "2024-01-14 14:30",
    duration: "11m 02s",
  },
];

const statusColor: Record<string, string> = {
  Running: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  Completed: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Failed: "bg-red-100 text-red-700 hover:bg-red-100",
};

const tierColor: Record<string, string> = {
  "Tier 0": "bg-slate-100 text-slate-700 hover:bg-slate-100",
  "Tier 1": "bg-purple-100 text-purple-700 hover:bg-purple-100",
  "Tier 2": "bg-indigo-100 text-indigo-700 hover:bg-indigo-100",
};

export default function RunsPage() {
  const [filter, setFilter] = useState("All");

  const filteredRuns =
    filter === "All" ? runs : runs.filter((r) => r.status === filter);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Runs</h1>
        <p className="text-sm text-slate-500">
          Track and monitor security validation runs
        </p>
      </div>

      <Tabs defaultValue="All" onValueChange={setFilter}>
        <TabsList>
          <TabsTrigger value="All">All</TabsTrigger>
          <TabsTrigger value="Running">Running</TabsTrigger>
          <TabsTrigger value="Completed">Completed</TabsTrigger>
          <TabsTrigger value="Failed">Failed</TabsTrigger>
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
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Run ID</TableHead>
                <TableHead>Engagement</TableHead>
                <TableHead>Tier</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Steps</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Duration</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredRuns.map((run, i) => (
                <TableRow key={i}>
                  <TableCell className="font-mono text-sm font-medium text-slate-900">
                    {run.id}
                  </TableCell>
                  <TableCell className="text-slate-600">
                    {run.engagement}
                  </TableCell>
                  <TableCell>
                    <Badge className={tierColor[run.tier]}>{run.tier}</Badge>
                  </TableCell>
                  <TableCell>
                    <Badge className={statusColor[run.status]}>
                      {run.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-600">
                    {run.stepsCompleted}/{run.stepsTotal}
                  </TableCell>
                  <TableCell className="text-slate-600">{run.started}</TableCell>
                  <TableCell className="text-slate-600">
                    {run.duration}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
