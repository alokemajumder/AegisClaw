"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Plus,
  Shield,
  MonitorSmartphone,
  UserCheck,
  Ticket,
  MessageSquare,
  Cloud,
  Settings,
  Zap,
} from "lucide-react";

const connectors = [
  {
    name: "Microsoft Sentinel",
    category: "SIEM",
    health: "healthy",
    description:
      "Cloud-native SIEM for intelligent security analytics across the enterprise.",
    icon: Shield,
  },
  {
    name: "Microsoft Defender",
    category: "EDR",
    health: "healthy",
    description:
      "Endpoint detection and response with advanced threat protection capabilities.",
    icon: MonitorSmartphone,
  },
  {
    name: "Entra ID",
    category: "Identity",
    health: "healthy",
    description:
      "Identity and access management for secure authentication and authorization.",
    icon: UserCheck,
  },
  {
    name: "ServiceNow",
    category: "ITSM",
    health: "degraded",
    description:
      "IT service management platform for incident ticketing and workflow automation.",
    icon: Ticket,
  },
  {
    name: "Microsoft Teams",
    category: "Notifications",
    health: "healthy",
    description:
      "Team collaboration and notification delivery for real-time alerting.",
    icon: MessageSquare,
  },
  {
    name: "Slack",
    category: "Notifications",
    health: "unhealthy",
    description:
      "Messaging platform integration for channel-based alert notifications.",
    icon: Cloud,
  },
];

const categories = [
  "All",
  "SIEM",
  "EDR",
  "ITSM",
  "Identity",
  "Notifications",
  "Cloud",
];

const healthDot: Record<string, string> = {
  healthy: "bg-emerald-500",
  degraded: "bg-yellow-500",
  unhealthy: "bg-red-500",
};

const healthLabel: Record<string, string> = {
  healthy: "Healthy",
  degraded: "Degraded",
  unhealthy: "Unhealthy",
};

const categoryColor: Record<string, string> = {
  SIEM: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  EDR: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  ITSM: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  Identity: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Notifications: "bg-indigo-100 text-indigo-700 hover:bg-indigo-100",
  Cloud: "bg-cyan-100 text-cyan-700 hover:bg-cyan-100",
};

export default function ConnectorsPage() {
  const [category, setCategory] = useState("All");

  const filteredConnectors =
    category === "All"
      ? connectors
      : connectors.filter((c) => c.category === category);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Connectors</h1>
          <p className="text-sm text-slate-500">
            Manage your security platform integrations
          </p>
        </div>
        <Button asChild>
          <a href="/connectors/new">
            <Plus className="h-4 w-4 mr-2" />
            Add Connector
          </a>
        </Button>
      </div>

      <Tabs defaultValue="All" onValueChange={setCategory}>
        <TabsList>
          {categories.map((cat) => (
            <TabsTrigger key={cat} value={cat}>
              {cat}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        {filteredConnectors.map((connector, i) => {
          const IconComponent = connector.icon;
          return (
            <Card key={i} className="flex flex-col">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-lg bg-slate-100">
                      <IconComponent className="h-5 w-5 text-slate-600" />
                    </div>
                    <div>
                      <CardTitle className="text-base text-slate-900">
                        {connector.name}
                      </CardTitle>
                      <Badge
                        className={`mt-1 text-[10px] ${categoryColor[connector.category]}`}
                      >
                        {connector.category}
                      </Badge>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <div
                      className={`h-2.5 w-2.5 rounded-full ${healthDot[connector.health]}`}
                    />
                    <span className="text-xs text-slate-500">
                      {healthLabel[connector.health]}
                    </span>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="flex-1 flex flex-col justify-between gap-4">
                <p className="text-sm text-slate-500">
                  {connector.description}
                </p>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" className="flex-1">
                    <Settings className="h-3.5 w-3.5 mr-1.5" />
                    Configure
                  </Button>
                  <Button variant="outline" size="sm" className="flex-1">
                    <Zap className="h-3.5 w-3.5 mr-1.5" />
                    Test
                  </Button>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
}
