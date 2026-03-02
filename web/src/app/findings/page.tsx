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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AlertTriangle, Filter } from "lucide-react";

const findings = [
  {
    title: "Defender alert latency >5min for PowerShell execution",
    severity: "Critical",
    confidence: "High",
    status: "Open",
    affectedAssets: 3,
    technique: "T1059.001",
    lastUpdated: "2 hours ago",
  },
  {
    title: "Kerberoasting attack not detected by Sentinel",
    severity: "Critical",
    confidence: "High",
    status: "Open",
    affectedAssets: 1,
    technique: "T1558.003",
    lastUpdated: "5 hours ago",
  },
  {
    title: "Lateral movement via PsExec partially detected",
    severity: "High",
    confidence: "Medium",
    status: "In Progress",
    affectedAssets: 4,
    technique: "T1570",
    lastUpdated: "1 day ago",
  },
  {
    title: "DNS tunneling exfiltration not blocked",
    severity: "Medium",
    confidence: "High",
    status: "Open",
    affectedAssets: 2,
    technique: "T1048.003",
    lastUpdated: "2 days ago",
  },
  {
    title: "Scheduled task persistence not flagged",
    severity: "Low",
    confidence: "Low",
    status: "Resolved",
    affectedAssets: 1,
    technique: "T1053.005",
    lastUpdated: "5 days ago",
  },
];

const severityColor: Record<string, string> = {
  Critical: "bg-red-100 text-red-700 hover:bg-red-100",
  High: "bg-orange-100 text-orange-700 hover:bg-orange-100",
  Medium: "bg-yellow-100 text-yellow-700 hover:bg-yellow-100",
  Low: "bg-blue-100 text-blue-700 hover:bg-blue-100",
};

const statusColor: Record<string, string> = {
  Open: "bg-red-100 text-red-700 hover:bg-red-100",
  "In Progress": "bg-amber-100 text-amber-700 hover:bg-amber-100",
  Resolved: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
};

export default function FindingsPage() {
  const [severityFilter, setSeverityFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");

  const filteredFindings = findings.filter((f) => {
    const matchSeverity =
      severityFilter === "all" || f.severity === severityFilter;
    const matchStatus = statusFilter === "all" || f.status === statusFilter;
    return matchSeverity && matchStatus;
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Findings</h1>
        <p className="text-sm text-slate-500">
          Security gaps identified during validation runs
        </p>
      </div>

      <div className="flex items-center gap-3">
        <Filter className="h-4 w-4 text-slate-400" />
        <Select value={severityFilter} onValueChange={setSeverityFilter}>
          <SelectTrigger className="w-[160px]">
            <SelectValue placeholder="Severity" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Severities</SelectItem>
            <SelectItem value="Critical">Critical</SelectItem>
            <SelectItem value="High">High</SelectItem>
            <SelectItem value="Medium">Medium</SelectItem>
            <SelectItem value="Low">Low</SelectItem>
          </SelectContent>
        </Select>
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-[160px]">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Statuses</SelectItem>
            <SelectItem value="Open">Open</SelectItem>
            <SelectItem value="In Progress">In Progress</SelectItem>
            <SelectItem value="Resolved">Resolved</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <AlertTriangle className="h-4 w-4" />
            Findings ({filteredFindings.length})
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead>Severity</TableHead>
                <TableHead>Confidence</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Affected Assets</TableHead>
                <TableHead>Technique</TableHead>
                <TableHead>Last Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredFindings.map((finding, i) => (
                <TableRow key={i}>
                  <TableCell className="font-medium text-slate-900 max-w-[300px]">
                    {finding.title}
                  </TableCell>
                  <TableCell>
                    <Badge className={severityColor[finding.severity]}>
                      {finding.severity}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-600">
                    {finding.confidence}
                  </TableCell>
                  <TableCell>
                    <Badge className={statusColor[finding.status]}>
                      {finding.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-600">
                    {finding.affectedAssets}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className="font-mono text-xs">
                      {finding.technique}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-500">
                    {finding.lastUpdated}
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
