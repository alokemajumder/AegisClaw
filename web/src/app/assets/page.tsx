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
import { Plus, Server } from "lucide-react";

const assets = [
  {
    name: "dc01.corp.local",
    type: "Domain Controller",
    criticality: "Critical",
    environment: "Production",
    owner: "Infrastructure Team",
    tags: ["Active Directory", "Tier 0"],
  },
  {
    name: "sentinel-workspace-01",
    type: "SIEM Workspace",
    criticality: "High",
    environment: "Production",
    owner: "SOC Team",
    tags: ["Monitoring", "SIEM"],
  },
  {
    name: "web-app-frontend",
    type: "Web Application",
    criticality: "Medium",
    environment: "Staging",
    owner: "Engineering",
    tags: ["Public-Facing", "React"],
  },
  {
    name: "k8s-cluster-prod",
    type: "Kubernetes Cluster",
    criticality: "High",
    environment: "Production",
    owner: "Platform Team",
    tags: ["Containers", "Orchestration"],
  },
  {
    name: "dev-jumpbox-01",
    type: "Virtual Machine",
    criticality: "Low",
    environment: "Development",
    owner: "DevOps",
    tags: ["Bastion", "SSH"],
  },
];

const criticalityColor: Record<string, string> = {
  Critical: "bg-red-100 text-red-700 hover:bg-red-100",
  High: "bg-orange-100 text-orange-700 hover:bg-orange-100",
  Medium: "bg-yellow-100 text-yellow-700 hover:bg-yellow-100",
  Low: "bg-blue-100 text-blue-700 hover:bg-blue-100",
};

const envColor: Record<string, string> = {
  Production: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Staging: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  Development: "bg-slate-100 text-slate-700 hover:bg-slate-100",
};

export default function AssetsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Assets</h1>
          <p className="text-sm text-slate-500">
            Inventory of assets under security validation
          </p>
        </div>
        <Button>
          <Plus className="h-4 w-4 mr-2" />
          Add Asset
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Server className="h-4 w-4" />
            Asset Inventory
          </CardTitle>
        </CardHeader>
        <CardContent>
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
              {assets.map((asset, i) => (
                <TableRow key={i}>
                  <TableCell className="font-medium text-slate-900">
                    {asset.name}
                  </TableCell>
                  <TableCell className="text-slate-600">{asset.type}</TableCell>
                  <TableCell>
                    <Badge className={criticalityColor[asset.criticality]}>
                      {asset.criticality}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge className={envColor[asset.environment]}>
                      {asset.environment}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-slate-600">{asset.owner}</TableCell>
                  <TableCell>
                    <div className="flex gap-1 flex-wrap">
                      {asset.tags.map((tag, j) => (
                        <Badge
                          key={j}
                          variant="outline"
                          className="text-[10px] px-1.5"
                        >
                          {tag}
                        </Badge>
                      ))}
                    </div>
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
