"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ShieldAlert, CheckCircle2, XCircle, Clock, Loader2 } from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listApprovals, approveRequest, denyRequest } from "@/lib/api";
import type { Approval } from "@/lib/types";

const statusColor: Record<string, string> = {
  pending: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  approved: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  denied: "bg-red-100 text-red-700 hover:bg-red-100",
  expired: "bg-slate-100 text-slate-700 hover:bg-slate-100",
};

export default function ApprovalsPage() {
  const { data: approvals, loading, error, refetch } = useApi<Approval[]>(() => listApprovals());
  const [acting, setActing] = useState<string | null>(null);

  const handleApprove = async (id: string) => {
    setActing(id);
    try {
      await approveRequest(id);
      refetch();
    } catch {
      // Error handled
    } finally {
      setActing(null);
    }
  };

  const handleDeny = async (id: string) => {
    setActing(id);
    try {
      await denyRequest(id);
      refetch();
    } catch {
      // Error handled
    } finally {
      setActing(null);
    }
  };

  const pendingCount = approvals?.filter((a) => a.status === "pending").length ?? 0;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Approvals</h1>
        <p className="text-sm text-slate-500">
          Review and approve pending security validation actions
        </p>
      </div>

      <div className="flex items-center gap-2 text-sm text-slate-500">
        <Clock className="h-4 w-4" />
        <span>{pendingCount} pending approval(s)</span>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-slate-500">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading approvals...
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
          Failed to load data: {error}
        </div>
      ) : (
        <div className="space-y-4">
          {approvals && approvals.length > 0 ? (
            approvals.map((approval) => (
              <Card key={approval.id}>
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div className="flex items-start gap-3">
                      <div className="p-2 rounded-lg bg-amber-50 mt-0.5">
                        <ShieldAlert className="h-5 w-5 text-amber-600" />
                      </div>
                      <div className="space-y-1">
                        <CardTitle className="text-base text-slate-900">
                          {approval.description}
                        </CardTitle>
                        <div className="flex items-center gap-2">
                          <Badge className={statusColor[approval.status] ?? "bg-slate-100 text-slate-700"}>
                            {approval.status}
                          </Badge>
                          <Badge variant="outline" className="text-xs">
                            {approval.request_type}
                          </Badge>
                          <span className="text-xs text-slate-400">
                            {approval.id.slice(0, 8)}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="flex items-center justify-between">
                    <div className="text-xs text-slate-400">
                      <span>Created: {new Date(approval.created_at).toLocaleString()}</span>
                      {approval.decided_at && (
                        <>
                          <span className="mx-2">|</span>
                          <span>Decided: {new Date(approval.decided_at).toLocaleString()}</span>
                        </>
                      )}
                    </div>
                    {approval.status === "pending" && (
                      <div className="flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleDeny(approval.id)}
                          disabled={acting === approval.id}
                        >
                          {acting === approval.id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <XCircle className="h-4 w-4 mr-1.5 text-red-500" />
                          )}
                          Deny
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => handleApprove(approval.id)}
                          disabled={acting === approval.id}
                        >
                          {acting === approval.id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <CheckCircle2 className="h-4 w-4 mr-1.5" />
                          )}
                          Approve
                        </Button>
                      </div>
                    )}
                  </div>
                </CardContent>
              </Card>
            ))
          ) : (
            <Card>
              <CardContent className="py-8 text-center text-slate-400">
                No approvals found
              </CardContent>
            </Card>
          )}
        </div>
      )}
    </div>
  );
}
