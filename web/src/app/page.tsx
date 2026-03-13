"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useApi } from "@/hooks/useApi";
import {
  getDashboardSummary,
  getDashboardActivity,
  getDashboardHealth,
} from "@/lib/api";
import type { DashboardSummary, DashboardHealth, AuditLogEntry } from "@/lib/types";
import { Loader2 } from "lucide-react";

export default function DashboardPage() {
  const { data: summary, loading: loadingSummary, error } = useApi<DashboardSummary>(
    getDashboardSummary
  );
  const { data: activity } = useApi<AuditLogEntry[]>(getDashboardActivity);
  const { data: health } = useApi<DashboardHealth>(getDashboardHealth);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Dashboard</h1>
        <p className="text-sm text-slate-500">
          Security validation posture overview
        </p>
      </div>

      {loadingSummary ? (
        <div className="flex items-center gap-2 text-slate-500">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading dashboard...
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
          Failed to load data: {error}
        </div>
      ) : (
        <>
          {/* Top metrics */}
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-slate-500">
                  Total Assets
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-3xl font-bold text-emerald-600">
                  {summary?.total_assets ?? 0}
                </div>
                <p className="text-xs text-slate-500 mt-1">
                  Under management
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-slate-500">
                  Active Runs
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-3xl font-bold text-blue-600">
                  {summary?.running_runs ?? 0}
                </div>
                <p className="text-xs text-slate-500 mt-1">
                  {summary?.active_engagements ?? 0} active engagements
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-slate-500">
                  Open Findings
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-3xl font-bold text-amber-600">
                  {summary?.total_findings ?? 0}
                </div>
                <div className="flex gap-1 mt-1 flex-wrap">
                  {(summary?.critical_findings ?? 0) > 0 && (
                    <Badge variant="destructive" className="text-[10px] px-1.5">
                      {summary!.critical_findings} Critical
                    </Badge>
                  )}
                  {(summary?.high_findings ?? 0) > 0 && (
                    <Badge className="text-[10px] px-1.5 bg-orange-100 text-orange-700 hover:bg-orange-100">
                      {summary!.high_findings} High
                    </Badge>
                  )}
                  {(summary?.medium_findings ?? 0) > 0 && (
                    <Badge className="text-[10px] px-1.5 bg-yellow-100 text-yellow-700 hover:bg-yellow-100">
                      {summary!.medium_findings} Medium
                    </Badge>
                  )}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm font-medium text-slate-500">
                  Coverage
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-3xl font-bold text-purple-600">
                  {summary?.coverage_entries ?? 0}
                </div>
                <p className="text-xs text-slate-500 mt-1">
                  {summary?.coverage_gaps ?? 0} gaps identified
                </p>
              </CardContent>
            </Card>
          </div>

          {/* Second row */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            {/* Recent Activity */}
            <Card className="lg:col-span-2">
              <CardHeader>
                <CardTitle className="text-base">Recent Activity</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {activity && activity.length > 0 ? (
                    activity.slice(0, 10).map((entry, i) => (
                      <div
                        key={i}
                        className="flex items-start gap-3 text-sm border-l-2 pl-3 py-1 border-slate-300"
                      >
                        <span className="text-slate-400 text-xs whitespace-nowrap w-28">
                          {new Date(entry.created_at).toLocaleString()}
                        </span>
                        <span className="text-slate-700">
                          {entry.action} — {entry.resource_type}
                          {entry.resource_id ? ` (${entry.resource_id.slice(0, 8)})` : ""}
                        </span>
                      </div>
                    ))
                  ) : (
                    <p className="text-sm text-slate-400">No recent activity</p>
                  )}
                </div>
              </CardContent>
            </Card>

            {/* System Health */}
            <Card>
              <CardHeader>
                <CardTitle className="text-base">System Health</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-slate-700">Database</span>
                    <Badge
                      className={
                        health?.database === "ok"
                          ? "bg-emerald-100 text-emerald-700 hover:bg-emerald-100"
                          : "bg-red-100 text-red-700 hover:bg-red-100"
                      }
                    >
                      {health?.database ?? "unknown"}
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-slate-700">NATS</span>
                    <Badge
                      className={
                        health?.nats === "ok"
                          ? "bg-emerald-100 text-emerald-700 hover:bg-emerald-100"
                          : "bg-amber-100 text-amber-700 hover:bg-amber-100"
                      }
                    >
                      {health?.nats ?? "unknown"}
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm text-slate-700">Kill Switch</span>
                    <Badge
                      className={
                        summary?.kill_switch_engaged
                          ? "bg-red-100 text-red-700 hover:bg-red-100"
                          : "bg-emerald-100 text-emerald-700 hover:bg-emerald-100"
                      }
                    >
                      {summary?.kill_switch_engaged ? "ENGAGED" : "Disengaged"}
                    </Badge>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
