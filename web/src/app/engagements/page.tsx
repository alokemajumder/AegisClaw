"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Plus, Target, Clock, Play } from "lucide-react";

const engagements = [
  {
    name: "Quarterly AD Validation",
    status: "Active",
    targetCount: 8,
    tierRange: "Tier 0 - Tier 2",
    schedule: "Every 7 days",
    lastRun: "2 hours ago",
    description:
      "Validates Active Directory attack paths including Kerberoasting, DCSync, and lateral movement techniques.",
  },
  {
    name: "Endpoint Detection Coverage",
    status: "Scheduled",
    targetCount: 12,
    tierRange: "Tier 0 - Tier 1",
    schedule: "Every 14 days",
    lastRun: "3 days ago",
    description:
      "Tests endpoint detection and response capabilities against common malware behaviors and living-off-the-land techniques.",
  },
  {
    name: "Cloud Security Posture",
    status: "Paused",
    targetCount: 5,
    tierRange: "Tier 1 - Tier 3",
    schedule: "Every 30 days",
    lastRun: "2 weeks ago",
    description:
      "Validates cloud security controls across Azure resources including identity, network, and data plane protections.",
  },
];

const statusColor: Record<string, string> = {
  Active: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Scheduled: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  Paused: "bg-slate-100 text-slate-700 hover:bg-slate-100",
};

export default function EngagementsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Engagements</h1>
          <p className="text-sm text-slate-500">
            Manage security validation engagements
          </p>
        </div>
        <Button>
          <Plus className="h-4 w-4 mr-2" />
          Create Engagement
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-4">
        {engagements.map((engagement, i) => (
          <Card key={i} className="flex flex-col">
            <CardHeader className="pb-3">
              <div className="flex items-start justify-between">
                <CardTitle className="text-base text-slate-900">
                  {engagement.name}
                </CardTitle>
                <Badge className={statusColor[engagement.status]}>
                  {engagement.status}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="flex-1 space-y-4">
              <p className="text-sm text-slate-500">{engagement.description}</p>

              <div className="space-y-2 text-sm">
                <div className="flex items-center gap-2 text-slate-600">
                  <Target className="h-4 w-4 text-slate-400" />
                  <span>{engagement.targetCount} targets</span>
                  <span className="text-slate-300">|</span>
                  <span>{engagement.tierRange}</span>
                </div>
                <div className="flex items-center gap-2 text-slate-600">
                  <Clock className="h-4 w-4 text-slate-400" />
                  <span>{engagement.schedule}</span>
                </div>
                <div className="flex items-center gap-2 text-slate-600">
                  <Play className="h-4 w-4 text-slate-400" />
                  <span>Last run: {engagement.lastRun}</span>
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
