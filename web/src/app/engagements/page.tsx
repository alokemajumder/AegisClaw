"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Plus, Target, Clock, Play, Loader2 } from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listEngagements, createEngagement, triggerRun } from "@/lib/api";
import type { Engagement } from "@/lib/types";

const statusColor: Record<string, string> = {
  active: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  draft: "bg-slate-100 text-slate-700 hover:bg-slate-100",
  paused: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  completed: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  archived: "bg-gray-100 text-gray-700 hover:bg-gray-100",
};

export default function EngagementsPage() {
  const { data: engagements, loading, error, refetch } = useApi<Engagement[]>(() => listEngagements());
  const [triggering, setTriggering] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [newEngagement, setNewEngagement] = useState({
    name: "",
    description: "",
    allowed_tiers: [0, 1] as number[],
    rate_limit: 60,
    concurrency_cap: 5,
  });

  const handleTierToggle = (tier: number) => {
    setNewEngagement((prev) => {
      const tiers = prev.allowed_tiers.includes(tier)
        ? prev.allowed_tiers.filter((t) => t !== tier)
        : [...prev.allowed_tiers, tier].sort();
      return { ...prev, allowed_tiers: tiers };
    });
  };

  const handleCreate = async () => {
    setSaving(true);
    try {
      await createEngagement(newEngagement);
      setDialogOpen(false);
      setNewEngagement({
        name: "",
        description: "",
        allowed_tiers: [0, 1],
        rate_limit: 60,
        concurrency_cap: 5,
      });
      refetch();
    } catch {
      // Error handled by API layer
    } finally {
      setSaving(false);
    }
  };

  const handleTrigger = async (id: string) => {
    setTriggering(id);
    try {
      await triggerRun(id);
    } catch {
      // Error handled by API layer
    } finally {
      setTriggering(null);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Engagements</h1>
          <p className="text-sm text-slate-500">
            Manage security validation engagements
          </p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Create Engagement
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Create Engagement</DialogTitle>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>Name</Label>
                <Input
                  value={newEngagement.name}
                  onChange={(e) => setNewEngagement({ ...newEngagement, name: e.target.value })}
                  placeholder="e.g., Q1 2026 Red Team Assessment"
                />
              </div>
              <div className="space-y-2">
                <Label>Description</Label>
                <Input
                  value={newEngagement.description}
                  onChange={(e) => setNewEngagement({ ...newEngagement, description: e.target.value })}
                  placeholder="Brief description of the engagement"
                />
              </div>
              <div className="space-y-2">
                <Label>Allowed Tiers</Label>
                <div className="flex gap-4">
                  {[0, 1, 2].map((tier) => (
                    <label key={tier} className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={newEngagement.allowed_tiers.includes(tier)}
                        onChange={() => handleTierToggle(tier)}
                        className="rounded border-slate-300"
                      />
                      Tier {tier}
                    </label>
                  ))}
                </div>
                <p className="text-xs text-slate-400">
                  Select which emulation tiers are permitted for this engagement.
                </p>
              </div>
              <div className="space-y-2">
                <Label>Rate Limit (req/min)</Label>
                <Input
                  type="number"
                  value={newEngagement.rate_limit}
                  onChange={(e) => setNewEngagement({ ...newEngagement, rate_limit: parseInt(e.target.value) || 0 })}
                  className="max-w-[200px]"
                />
              </div>
              <div className="space-y-2">
                <Label>Concurrency Cap</Label>
                <Input
                  type="number"
                  value={newEngagement.concurrency_cap}
                  onChange={(e) => setNewEngagement({ ...newEngagement, concurrency_cap: parseInt(e.target.value) || 0 })}
                  className="max-w-[200px]"
                />
              </div>
              <Button onClick={handleCreate} disabled={saving || !newEngagement.name} className="w-full">
                {saving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Create Engagement
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-slate-500">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading engagements...
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
          Failed to load data: {error}
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-4">
          {engagements && engagements.length > 0 ? (
            engagements.map((engagement) => (
              <Card key={engagement.id} className="flex flex-col">
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <CardTitle className="text-base text-slate-900">
                      {engagement.name}
                    </CardTitle>
                    <Badge className={statusColor[engagement.status] ?? "bg-slate-100 text-slate-700"}>
                      {engagement.status}
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent className="flex-1 space-y-4">
                  {engagement.description && (
                    <p className="text-sm text-slate-500">{engagement.description}</p>
                  )}

                  <div className="space-y-2 text-sm">
                    <div className="flex items-center gap-2 text-slate-600">
                      <Target className="h-4 w-4 text-slate-400" />
                      <span>{engagement.target_allowlist?.length ?? 0} targets</span>
                      <span className="text-slate-300">|</span>
                      <span>Tiers: {engagement.allowed_tiers?.join(", ") ?? "—"}</span>
                    </div>
                    {engagement.schedule_cron && (
                      <div className="flex items-center gap-2 text-slate-600">
                        <Clock className="h-4 w-4 text-slate-400" />
                        <span>{engagement.schedule_cron}</span>
                      </div>
                    )}
                  </div>

                  {engagement.status === "active" && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="w-full"
                      onClick={() => handleTrigger(engagement.id)}
                      disabled={triggering === engagement.id}
                    >
                      {triggering === engagement.id ? (
                        <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                      ) : (
                        <Play className="h-4 w-4 mr-2" />
                      )}
                      Trigger Run
                    </Button>
                  )}
                </CardContent>
              </Card>
            ))
          ) : (
            <Card className="col-span-full">
              <CardContent className="py-8 text-center text-slate-400">
                No engagements found. Click &quot;Create Engagement&quot; to get started.
              </CardContent>
            </Card>
          )}
        </div>
      )}
    </div>
  );
}
