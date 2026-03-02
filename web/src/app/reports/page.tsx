"use client";

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
import { Plus, FileText, Download } from "lucide-react";

const reports = [
  {
    name: "Q4 2024 Executive Summary",
    type: "Executive",
    date: "2024-01-15",
    format: "PDF",
    size: "2.4 MB",
    engagement: "Quarterly AD Validation",
  },
  {
    name: "Endpoint Detection Technical Report",
    type: "Technical",
    date: "2024-01-12",
    format: "PDF",
    size: "8.1 MB",
    engagement: "Endpoint Detection Coverage",
  },
  {
    name: "ATT&CK Coverage Matrix",
    type: "Coverage",
    date: "2024-01-10",
    format: "CSV",
    size: "156 KB",
    engagement: "All Engagements",
  },
];

const typeColor: Record<string, string> = {
  Executive: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  Technical: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  Coverage: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
};

export default function ReportsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Reports</h1>
          <p className="text-sm text-slate-500">
            Generate and download security validation reports
          </p>
        </div>
        <Button>
          <Plus className="h-4 w-4 mr-2" />
          Generate Report
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <FileText className="h-4 w-4" />
            Generated Reports
          </CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Report Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Engagement</TableHead>
                <TableHead>Date</TableHead>
                <TableHead>Format</TableHead>
                <TableHead>Size</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {reports.map((report, i) => (
                <TableRow key={i}>
                  <TableCell className="font-medium text-slate-900">
                    {report.name}
                  </TableCell>
                  <TableCell>
                    <Badge className={typeColor[report.type]}>
                      {report.type}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-600">
                    {report.engagement}
                  </TableCell>
                  <TableCell className="text-slate-600">{report.date}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="text-xs">
                      {report.format}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-500">{report.size}</TableCell>
                  <TableCell className="text-right">
                    <Button variant="ghost" size="sm">
                      <Download className="h-4 w-4 mr-1.5" />
                      Download
                    </Button>
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
