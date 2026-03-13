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
import { Plus, FileText, Download, Loader2 } from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listReports, generateReport, getReportDownloadUrl } from "@/lib/api";
import type { Report } from "@/lib/types";

const typeColor: Record<string, string> = {
  executive: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  technical: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  coverage: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  compliance: "bg-amber-100 text-amber-700 hover:bg-amber-100",
};

const statusColor: Record<string, string> = {
  generating: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  completed: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  failed: "bg-red-100 text-red-700 hover:bg-red-100",
};

export default function ReportsPage() {
  const { data: reports, loading, error, refetch } = useApi<Report[]>(() => listReports());
  const [dialogOpen, setDialogOpen] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [newReport, setNewReport] = useState({
    title: "",
    type: "executive",
    format: "markdown",
  });

  const handleGenerate = async () => {
    setGenerating(true);
    try {
      await generateReport(newReport);
      setDialogOpen(false);
      setNewReport({ title: "", type: "executive", format: "markdown" });
      refetch();
    } catch {
      // Error handled by API layer
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Reports</h1>
          <p className="text-sm text-slate-500">
            Generate and download security validation reports
          </p>
        </div>
        <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
          <DialogTrigger asChild>
            <Button>
              <Plus className="h-4 w-4 mr-2" />
              Generate Report
            </Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Generate Report</DialogTitle>
            </DialogHeader>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label>Title</Label>
                <Input
                  value={newReport.title}
                  onChange={(e) => setNewReport({ ...newReport, title: e.target.value })}
                  placeholder="e.g., Q1 2026 Executive Summary"
                />
              </div>
              <div className="space-y-2">
                <Label>Report Type</Label>
                <select
                  className="w-full border rounded-md px-3 py-2 text-sm"
                  value={newReport.type}
                  onChange={(e) => setNewReport({ ...newReport, type: e.target.value })}
                >
                  <option value="executive">Executive</option>
                  <option value="technical">Technical</option>
                  <option value="coverage">Coverage</option>
                  <option value="compliance">Compliance</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label>Format</Label>
                <select
                  className="w-full border rounded-md px-3 py-2 text-sm"
                  value={newReport.format}
                  onChange={(e) => setNewReport({ ...newReport, format: e.target.value })}
                >
                  <option value="markdown">Markdown</option>
                  <option value="json">JSON</option>
                </select>
              </div>
              <Button onClick={handleGenerate} disabled={generating || !newReport.title} className="w-full">
                {generating ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Generate
              </Button>
            </div>
          </DialogContent>
        </Dialog>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <FileText className="h-4 w-4" />
            Generated Reports
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex items-center gap-2 text-slate-500 py-8 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading reports...
            </div>
          ) : error ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
              Failed to load data: {error}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Report Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Format</TableHead>
                  <TableHead>Date</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {reports && reports.length > 0 ? (
                  reports.map((report) => (
                    <TableRow key={report.id}>
                      <TableCell className="font-medium text-slate-900">
                        {report.title}
                      </TableCell>
                      <TableCell>
                        <Badge className={typeColor[report.report_type] ?? "bg-slate-100 text-slate-700"}>
                          {report.report_type}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge className={statusColor[report.status] ?? "bg-slate-100 text-slate-700"}>
                          {report.status}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-xs">
                          {report.format}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-slate-600">
                        {new Date(report.created_at).toLocaleDateString()}
                      </TableCell>
                      <TableCell className="text-right">
                        {report.status === "completed" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => window.open(getReportDownloadUrl(report.id), "_blank")}
                          >
                            <Download className="h-4 w-4 mr-1.5" />
                            Download
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={6} className="text-center text-slate-400 py-8">
                      No reports generated yet. Click &quot;Generate Report&quot; to create one.
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
