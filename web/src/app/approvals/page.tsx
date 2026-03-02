"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ShieldAlert, CheckCircle2, XCircle, Clock } from "lucide-react";

const pendingApprovals = [
  {
    id: "APR-101",
    description:
      "Execute Tier 2 credential dumping emulation (T1003.001 — LSASS Memory) against dc01.corp.local",
    tier: "Tier 2",
    requestedBy: "Engagement: Quarterly AD Validation",
    requestedTime: "10 minutes ago",
    risk: "Medium",
    details:
      "This step will attempt to read LSASS process memory using procdump. The action is reversible and scoped to the target host only.",
  },
  {
    id: "APR-100",
    description:
      "Initiate Tier 3 network-level lateral movement test (T1021.002 — SMB/Windows Admin Shares) across 3 hosts",
    tier: "Tier 3",
    requestedBy: "Engagement: Endpoint Detection Coverage",
    requestedTime: "25 minutes ago",
    risk: "High",
    details:
      "This step will attempt authenticated SMB connections to target hosts using provided service account. Requires explicit approval due to Tier 3 classification.",
  },
];

const tierColor: Record<string, string> = {
  "Tier 0": "bg-slate-100 text-slate-700 hover:bg-slate-100",
  "Tier 1": "bg-blue-100 text-blue-700 hover:bg-blue-100",
  "Tier 2": "bg-purple-100 text-purple-700 hover:bg-purple-100",
  "Tier 3": "bg-red-100 text-red-700 hover:bg-red-100",
};

const riskColor: Record<string, string> = {
  Low: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  Medium: "bg-yellow-100 text-yellow-700 hover:bg-yellow-100",
  High: "bg-orange-100 text-orange-700 hover:bg-orange-100",
};

export default function ApprovalsPage() {
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
        <span>{pendingApprovals.length} pending approval(s)</span>
      </div>

      <div className="space-y-4">
        {pendingApprovals.map((approval, i) => (
          <Card key={i}>
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
                      <Badge className={tierColor[approval.tier]}>
                        {approval.tier}
                      </Badge>
                      <Badge className={riskColor[approval.risk]}>
                        Risk: {approval.risk}
                      </Badge>
                      <span className="text-xs text-slate-400">
                        {approval.id}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm text-slate-500">{approval.details}</p>

              <div className="flex items-center justify-between">
                <div className="text-xs text-slate-400">
                  <span>Requested by: {approval.requestedBy}</span>
                  <span className="mx-2">|</span>
                  <span>{approval.requestedTime}</span>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm">
                    <XCircle className="h-4 w-4 mr-1.5 text-red-500" />
                    Deny
                  </Button>
                  <Button size="sm">
                    <CheckCircle2 className="h-4 w-4 mr-1.5" />
                    Approve
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
