"use client";

import { use, useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ArrowLeft, Loader2, AlertTriangle, ExternalLink, Ticket } from "lucide-react";
import Link from "next/link";
import { useApi } from "@/hooks/useApi";
import { getFinding, createFindingTicket, listConnectorInstances } from "@/lib/api";
import type { Finding, ConnectorInstance } from "@/lib/types";

const severityColor: Record<string, string> = {
  critical: "bg-red-100 text-red-700",
  high: "bg-orange-100 text-orange-700",
  medium: "bg-amber-100 text-amber-700",
  low: "bg-emerald-100 text-emerald-700",
  informational: "bg-slate-100 text-slate-700",
};

const confidenceColor: Record<string, string> = {
  high: "bg-emerald-100 text-emerald-700",
  medium: "bg-amber-100 text-amber-700",
  low: "bg-slate-100 text-slate-700",
};

export default function FindingDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { data: finding, loading, error, refetch } = useApi<Finding>(() => getFinding(id).then(r => ({ data: r.data })), [id]);
  const [ticketing, setTicketing] = useState(false);
  const [itsmConnectors, setItsmConnectors] = useState<ConnectorInstance[]>([]);
  const [selectedConnectorId, setSelectedConnectorId] = useState<string>("");
  const [connectorsLoading, setConnectorsLoading] = useState(true);

  useEffect(() => {
    listConnectorInstances(1, 200)
      .then((resp) => {
        const itsm = (resp.data ?? []).filter(
          (c: ConnectorInstance) => c.category === "itsm" && c.enabled
        );
        setItsmConnectors(itsm);
        if (itsm.length === 1) {
          setSelectedConnectorId(itsm[0].id);
        }
      })
      .catch(() => {
        setItsmConnectors([]);
      })
      .finally(() => setConnectorsLoading(false));
  }, []);

  const handleCreateTicket = async () => {
    if (!selectedConnectorId) return;
    setTicketing(true);
    try {
      await createFindingTicket(id, { connector_id: selectedConnectorId });
      refetch();
    } catch { /* handled */ } finally {
      setTicketing(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center gap-2 text-slate-500 py-12 justify-center">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading finding...
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

  if (!finding) {
    return <div className="text-center py-12 text-slate-400">Finding not found</div>;
  }

  const renderTicketButton = () => {
    if (finding.ticket_id) return null;
    if (connectorsLoading) {
      return <Loader2 className="h-4 w-4 animate-spin text-slate-400" />;
    }
    if (itsmConnectors.length === 0) {
      return (
        <span className="text-sm text-slate-400">No ITSM connector configured</span>
      );
    }
    return (
      <div className="flex items-center gap-2">
        {itsmConnectors.length > 1 && (
          <Select value={selectedConnectorId} onValueChange={setSelectedConnectorId}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Select connector" />
            </SelectTrigger>
            <SelectContent>
              {itsmConnectors.map((c) => (
                <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        <Button size="sm" onClick={handleCreateTicket} disabled={ticketing || !selectedConnectorId}>
          {ticketing ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : <Ticket className="h-4 w-4 mr-1" />}
          Create Ticket
        </Button>
      </div>
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/findings">
            <Button variant="ghost" size="sm" aria-label="Back to findings"><ArrowLeft className="h-4 w-4" /></Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold text-slate-900 flex items-center gap-2">
              <AlertTriangle className="h-5 w-5" />
              {finding.title}
            </h1>
            <p className="text-sm text-slate-500">{finding.id.slice(0, 8)}</p>
          </div>
        </div>
        <div className="flex gap-2">
          {(finding.ticket_url || (finding.metadata?.ticket_url as string)) && (
            <a href={(finding.ticket_url || finding.metadata?.ticket_url) as string} target="_blank" rel="noopener noreferrer">
              <Button variant="outline" size="sm">
                <ExternalLink className="h-4 w-4 mr-1" /> View Ticket
              </Button>
            </a>
          )}
          {renderTicketButton()}
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Severity</CardTitle></CardHeader>
          <CardContent>
            <Badge className={severityColor[finding.severity]}>{finding.severity}</Badge>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Confidence</CardTitle></CardHeader>
          <CardContent>
            <Badge className={confidenceColor[finding.confidence]}>{finding.confidence}</Badge>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Status</CardTitle></CardHeader>
          <CardContent>
            <Badge variant="outline">{finding.status}</Badge>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-sm text-slate-500">Discovered</CardTitle></CardHeader>
          <CardContent>
            <span className="text-sm">{new Date(finding.created_at).toLocaleDateString()}</span>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Description</CardTitle></CardHeader>
        <CardContent>
          <p className="text-sm text-slate-700 whitespace-pre-wrap">{finding.description || "No description provided."}</p>
        </CardContent>
      </Card>

      {finding.remediation && (
        <Card>
          <CardHeader><CardTitle className="text-base">Remediation</CardTitle></CardHeader>
          <CardContent>
            <p className="text-sm text-slate-700 whitespace-pre-wrap">{finding.remediation}</p>
          </CardContent>
        </Card>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {finding.technique_ids && finding.technique_ids.length > 0 && (
          <Card>
            <CardHeader><CardTitle className="text-base">MITRE ATT&CK Techniques</CardTitle></CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-2">
                {finding.technique_ids.map(t => (
                  <Badge key={t} variant="outline">{t}</Badge>
                ))}
              </div>
            </CardContent>
          </Card>
        )}

        {finding.evidence_ids && finding.evidence_ids.length > 0 && (
          <Card>
            <CardHeader><CardTitle className="text-base">Evidence</CardTitle></CardHeader>
            <CardContent>
              <div className="space-y-1">
                {finding.evidence_ids.map(e => (
                  <div key={e} className="text-sm text-slate-600 font-mono">{e}</div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      <Card>
        <CardHeader><CardTitle className="text-base">Metadata</CardTitle></CardHeader>
        <CardContent className="space-y-2 text-sm">
          {finding.run_id && (
            <div className="flex justify-between">
              <span className="text-slate-500">Run</span>
              <Link href={`/runs/${finding.run_id}`} className="text-blue-600 hover:underline">{finding.run_id.slice(0, 8)}</Link>
            </div>
          )}
          {finding.ticket_id && (
            <div className="flex justify-between">
              <span className="text-slate-500">Ticket</span>
              <span>{finding.ticket_id}</span>
            </div>
          )}
          {finding.cluster_id && (
            <div className="flex justify-between">
              <span className="text-slate-500">Dedup Cluster</span>
              <span className="font-mono text-xs">{finding.cluster_id.slice(0, 12)}</span>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
