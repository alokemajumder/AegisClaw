"use client";

import { use } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { ArrowLeft, Server, Loader2, AlertTriangle } from "lucide-react";
import Link from "next/link";
import { useApi } from "@/hooks/useApi";
import { getAsset, listAssetFindings } from "@/lib/api";
import type { Asset, Finding } from "@/lib/types";

const severityColor: Record<string, string> = {
  critical: "bg-red-100 text-red-700",
  high: "bg-orange-100 text-orange-700",
  medium: "bg-amber-100 text-amber-700",
  low: "bg-emerald-100 text-emerald-700",
  informational: "bg-slate-100 text-slate-700",
};

export default function AssetDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: asset, loading, error } = useApi<Asset>(() => getAsset(id).then(r => ({ data: r.data })), [id]);
  const { data: findings } = useApi<Finding[]>(() => listAssetFindings(id).then(r => ({ data: r.data })), [id]);

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-slate-500 py-12 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading asset...
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

  if (!asset) {
    return <div className="text-center py-12 text-slate-400">Asset not found</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/assets">
          <Button variant="ghost" size="sm" aria-label="Back to assets"><ArrowLeft className="h-4 w-4" /></Button>
        </Link>
        <div>
          <h1 className="text-2xl font-bold text-slate-900">{asset.name}</h1>
          <p className="text-sm text-slate-500">{asset.hostname || asset.ip_address || "No hostname"}</p>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Type</CardTitle></CardHeader>
          <CardContent>
            <div className="flex items-center gap-2">
              <Server className="h-4 w-4 text-slate-400" />
              <Badge variant="outline">{asset.asset_type}</Badge>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Environment</CardTitle></CardHeader>
          <CardContent>
            <span className="text-lg font-semibold">{asset.environment || "—"}</span>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Criticality</CardTitle></CardHeader>
          <CardContent>
            <Badge className={asset.criticality === "critical" ? "bg-red-100 text-red-700" : asset.criticality === "high" ? "bg-orange-100 text-orange-700" : "bg-slate-100 text-slate-700"}>
              {asset.criticality || "—"}
            </Badge>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <CardTitle className="text-sm text-slate-500">Details</CardTitle>
          </div>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex justify-between"><span className="text-slate-500">Owner</span><span>{asset.owner || "—"}</span></div>
          <div className="flex justify-between"><span className="text-slate-500">OS</span><span>{asset.os || "—"}</span></div>
          <div className="flex justify-between"><span className="text-slate-500">IP Address</span><span>{asset.ip_address || "—"}</span></div>
          <div className="flex justify-between"><span className="text-slate-500">Created</span><span>{new Date(asset.created_at).toLocaleDateString()}</span></div>
          {asset.tags && asset.tags.length > 0 && (
            <div className="flex justify-between">
              <span className="text-slate-500">Tags</span>
              <div className="flex gap-1">{asset.tags.map(t => <Badge key={t} variant="outline" className="text-xs">{t}</Badge>)}</div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <AlertTriangle className="h-4 w-4" />
            Findings ({findings?.length ?? 0})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {findings && findings.length > 0 ? (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Title</TableHead>
                  <TableHead>Severity</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Date</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {findings.map(f => (
                  <TableRow key={f.id}>
                    <TableCell>
                      <Link href={`/findings/${f.id}`} className="text-blue-600 hover:underline">{f.title}</Link>
                    </TableCell>
                    <TableCell><Badge className={severityColor[f.severity] ?? "bg-slate-100 text-slate-700"}>{f.severity}</Badge></TableCell>
                    <TableCell><Badge variant="outline">{f.status}</Badge></TableCell>
                    <TableCell className="text-slate-500 text-xs">{new Date(f.created_at).toLocaleDateString()}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <p className="text-center text-slate-400 py-6">No findings for this asset</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
