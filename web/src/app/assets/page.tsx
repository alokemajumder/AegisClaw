"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Plus, Server, Loader2 } from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listAssets, createAsset } from "@/lib/api";
import type { Asset } from "@/lib/types";

const criticalityColor: Record<string, string> = {
  critical: "bg-red-100 text-red-700 hover:bg-red-100",
  high: "bg-orange-100 text-orange-700 hover:bg-orange-100",
  medium: "bg-yellow-100 text-yellow-700 hover:bg-yellow-100",
  low: "bg-blue-100 text-blue-700 hover:bg-blue-100",
};

const envColor: Record<string, string> = {
  production: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  staging: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  development: "bg-slate-100 text-slate-700 hover:bg-slate-100",
};

export default function AssetsPage() {
  const { data: assets, loading, error, refetch } = useApi<Asset[]>(() => listAssets());
  const [dialogOpen, setDialogOpen] = useState(false);
  const [newAsset, setNewAsset] = useState({
    name: "",
    asset_type: "server",
    hostname: "",
    environment: "production",
    criticality: "medium",
    owner: "",
  });
  const [saving, setSaving] = useState(false);

  const handleCreate = async () => {
    setSaving(true);
    try {
      await createAsset(newAsset);
      setDialogOpen(false);
      setNewAsset({ name: "", asset_type: "server", hostname: "", environment: "production", criticality: "medium", owner: "" });
      refetch();
    } catch {
      // Error handled by API layer
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Assets</h1>
          <p className="text-sm text-slate-500">
            Inventory of assets under security validation
          </p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Add Asset
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Add Asset</DialogTitle>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>Name</Label>
                <Input value={newAsset.name} onChange={(e) => setNewAsset({ ...newAsset, name: e.target.value })} placeholder="e.g., dc01.corp.local" />
              </div>
              <div className="space-y-2">
                <Label>Type</Label>
                <select className="w-full border rounded-md px-3 py-2 text-sm" value={newAsset.asset_type} onChange={(e) => setNewAsset({ ...newAsset, asset_type: e.target.value })}>
                  <option value="server">Server</option>
                  <option value="endpoint">Endpoint</option>
                  <option value="cloud_resource">Cloud Resource</option>
                  <option value="application">Application</option>
                  <option value="network_device">Network Device</option>
                  <option value="identity">Identity</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label>Hostname</Label>
                <Input value={newAsset.hostname} onChange={(e) => setNewAsset({ ...newAsset, hostname: e.target.value })} placeholder="hostname or IP" />
              </div>
              <div className="space-y-2">
                <Label>Environment</Label>
                <select className="w-full border rounded-md px-3 py-2 text-sm" value={newAsset.environment} onChange={(e) => setNewAsset({ ...newAsset, environment: e.target.value })}>
                  <option value="production">Production</option>
                  <option value="staging">Staging</option>
                  <option value="development">Development</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label>Criticality</Label>
                <select className="w-full border rounded-md px-3 py-2 text-sm" value={newAsset.criticality} onChange={(e) => setNewAsset({ ...newAsset, criticality: e.target.value })}>
                  <option value="critical">Critical</option>
                  <option value="high">High</option>
                  <option value="medium">Medium</option>
                  <option value="low">Low</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label>Owner</Label>
                <Input value={newAsset.owner} onChange={(e) => setNewAsset({ ...newAsset, owner: e.target.value })} placeholder="Team or person" />
              </div>
              <Button onClick={handleCreate} disabled={saving || !newAsset.name} className="w-full">
                {saving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Create Asset
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Server className="h-4 w-4" />
            Asset Inventory
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex items-center gap-2 text-slate-500 py-8 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading assets...
            </div>
          ) : error ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
              Failed to load data: {error}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Criticality</TableHead>
                  <TableHead>Environment</TableHead>
                  <TableHead>Owner</TableHead>
                  <TableHead>Tags</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {assets && assets.length > 0 ? (
                  assets.map((asset) => (
                    <TableRow key={asset.id}>
                      <TableCell className="font-medium text-slate-900">
                        {asset.name}
                      </TableCell>
                      <TableCell className="text-slate-600">{asset.asset_type}</TableCell>
                      <TableCell>
                        <Badge className={criticalityColor[asset.criticality ?? "medium"] ?? "bg-slate-100 text-slate-700"}>
                          {asset.criticality ?? "medium"}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge className={envColor[asset.environment ?? "production"] ?? "bg-slate-100 text-slate-700"}>
                          {asset.environment ?? "—"}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-slate-600">{asset.owner ?? "—"}</TableCell>
                      <TableCell>
                        <div className="flex gap-1 flex-wrap">
                          {(asset.tags ?? []).map((tag, j) => (
                            <Badge key={j} variant="outline" className="text-[10px] px-1.5">
                              {tag}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-slate-400 py-8">
                      No assets found. Click &quot;Add Asset&quot; to get started.
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
