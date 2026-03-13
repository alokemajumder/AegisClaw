"use client";

import { useState, useEffect } from "react";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AlertTriangle, Filter, Loader2, Ticket } from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listFindings, createFindingTicket, listConnectorInstances } from "@/lib/api";
import type { Finding, ConnectorInstance } from "@/lib/types";

const severityColor: Record<string, string> = {
  critical: "bg-red-100 text-red-700 hover:bg-red-100",
  high: "bg-orange-100 text-orange-700 hover:bg-orange-100",
  medium: "bg-yellow-100 text-yellow-700 hover:bg-yellow-100",
  low: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  informational: "bg-slate-100 text-slate-700 hover:bg-slate-100",
};

const statusColor: Record<string, string> = {
  observed: "bg-slate-100 text-slate-700 hover:bg-slate-100",
  needs_review: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  confirmed: "bg-red-100 text-red-700 hover:bg-red-100",
  ticketed: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  fixed: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  closed: "bg-slate-100 text-slate-700 hover:bg-slate-100",
};

export default function FindingsPage() {
  const [severityFilter, setSeverityFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");

  const { data: findings, loading, error, refetch } = useApi<Finding[]>(
    () => listFindings(
      1, 50,
      severityFilter === "all" ? undefined : severityFilter,
      statusFilter === "all" ? undefined : statusFilter
    ),
    [severityFilter, statusFilter]
  );

  // ITSM connector state
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

  const [ticketing, setTicketing] = useState<string | null>(null);
  const handleCreateTicket = async (findingId: string) => {
    if (!selectedConnectorId) return;
    setTicketing(findingId);
    try {
      await createFindingTicket(findingId, { connector_id: selectedConnectorId });
      refetch();
    } catch {
      // Error handled by API layer
    } finally {
      setTicketing(null);
    }
  };

  const hasItsmConnector = !connectorsLoading && itsmConnectors.length > 0;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Findings</h1>
        <p className="text-sm text-slate-500">
          Security gaps identified during validation runs
        </p>
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <Filter className="h-4 w-4 text-slate-400" />
        <Select value={severityFilter} onValueChange={setSeverityFilter}>
          <SelectTrigger className="w-[160px]">
            <SelectValue placeholder="Severity" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Severities</SelectItem>
            <SelectItem value="critical">Critical</SelectItem>
            <SelectItem value="high">High</SelectItem>
            <SelectItem value="medium">Medium</SelectItem>
            <SelectItem value="low">Low</SelectItem>
            <SelectItem value="informational">Informational</SelectItem>
          </SelectContent>
        </Select>
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-[160px]">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Statuses</SelectItem>
            <SelectItem value="observed">Observed</SelectItem>
            <SelectItem value="needs_review">Needs Review</SelectItem>
            <SelectItem value="confirmed">Confirmed</SelectItem>
            <SelectItem value="ticketed">Ticketed</SelectItem>
            <SelectItem value="fixed">Fixed</SelectItem>
            <SelectItem value="closed">Closed</SelectItem>
          </SelectContent>
        </Select>
        {!connectorsLoading && itsmConnectors.length > 1 && (
          <Select value={selectedConnectorId} onValueChange={setSelectedConnectorId}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="ITSM Connector" />
            </SelectTrigger>
            <SelectContent>
              {itsmConnectors.map((c) => (
                <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
        {!connectorsLoading && itsmConnectors.length === 0 && (
          <span className="text-sm text-slate-400">No ITSM connector configured</span>
        )}
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <AlertTriangle className="h-4 w-4" />
            Findings ({findings?.length ?? 0})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex items-center gap-2 text-slate-500 py-8 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading findings...
            </div>
          ) : error ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
              Failed to load data: {error}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Title</TableHead>
                  <TableHead>Severity</TableHead>
                  <TableHead>Confidence</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Techniques</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {findings && findings.length > 0 ? (
                  findings.map((finding) => (
                    <TableRow key={finding.id}>
                      <TableCell className="font-medium text-slate-900 max-w-[300px]">
                        {finding.title}
                      </TableCell>
                      <TableCell>
                        <Badge className={severityColor[finding.severity] ?? "bg-slate-100 text-slate-700"}>
                          {finding.severity}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-slate-600">
                        {finding.confidence}
                      </TableCell>
                      <TableCell>
                        <Badge className={statusColor[finding.status] ?? "bg-slate-100 text-slate-700"}>
                          {finding.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex gap-1 flex-wrap">
                          {(finding.technique_ids ?? []).map((t, j) => (
                            <Badge key={j} variant="outline" className="font-mono text-xs">
                              {t}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell>
                        {!finding.ticket_id && hasItsmConnector && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleCreateTicket(finding.id)}
                            disabled={ticketing === finding.id || !selectedConnectorId}
                          >
                            {ticketing === finding.id ? (
                              <Loader2 className="h-3.5 w-3.5 animate-spin" />
                            ) : (
                              <Ticket className="h-3.5 w-3.5 mr-1" />
                            )}
                            Create Ticket
                          </Button>
                        )}
                        {finding.ticket_url && (
                          <a href={finding.ticket_url} target="_blank" rel="noopener noreferrer" className="text-sm text-blue-600 hover:underline">
                            {finding.ticket_id}
                          </a>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-slate-400 py-8">
                      No findings found
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
