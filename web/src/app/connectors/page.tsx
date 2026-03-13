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
  Loader2,
} from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listConnectorInstances, testConnector } from "@/lib/api";
import type { ConnectorInstance } from "@/lib/types";

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
  unknown: "bg-slate-400",
};

const categoryColor: Record<string, string> = {
  SIEM: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  EDR: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  ITSM: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  Identity: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Notifications: "bg-indigo-100 text-indigo-700 hover:bg-indigo-100",
  Cloud: "bg-cyan-100 text-cyan-700 hover:bg-cyan-100",
};

const iconMap: Record<string, typeof Shield> = {
  SIEM: Shield,
  EDR: MonitorSmartphone,
  Identity: UserCheck,
  ITSM: Ticket,
  Notifications: MessageSquare,
  Cloud: Cloud,
};

export default function ConnectorsPage() {
  const [category, setCategory] = useState("All");
  const { data: connectors, loading, error } = useApi<ConnectorInstance[]>(() => listConnectorInstances());
  const [testing, setTesting] = useState<string | null>(null);

  const handleTest = async (id: string) => {
    setTesting(id);
    try {
      await testConnector(id);
    } catch {
      // Error handled by API
    } finally {
      setTesting(null);
    }
  };

  // We don't have category on ConnectorInstance directly from API,
  // so we show all when no filter or filter by name containing the category
  const filteredConnectors = connectors
    ? category === "All"
      ? connectors
      : connectors.filter((c) => c.name.toLowerCase().includes(category.toLowerCase()))
    : [];

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

      {loading ? (
        <div className="flex items-center gap-2 text-slate-500">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading connectors...
        </div>
      ) : error ? (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
          Failed to load data: {error}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredConnectors.length > 0 ? (
            filteredConnectors.map((connector) => {
              const IconComponent = iconMap["SIEM"] ?? Shield;
              return (
                <Card key={connector.id} className="flex flex-col">
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
                          <Badge className={`mt-1 text-[10px] ${categoryColor["SIEM"] ?? "bg-slate-100 text-slate-700"}`}>
                            Connector
                          </Badge>
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <div className={`h-2.5 w-2.5 rounded-full ${healthDot[connector.health_status] ?? healthDot["unknown"]}`} />
                        <span className="text-xs text-slate-500">
                          {connector.health_status}
                        </span>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent className="flex-1 flex flex-col justify-between gap-4">
                    <p className="text-sm text-slate-500">
                      {connector.description ?? "No description"}
                    </p>
                    <div className="flex items-center gap-2 text-xs text-slate-400">
                      <span>{connector.enabled ? "Enabled" : "Disabled"}</span>
                      {connector.last_health_check && (
                        <>
                          <span className="text-slate-300">|</span>
                          <span>Last check: {new Date(connector.last_health_check).toLocaleString()}</span>
                        </>
                      )}
                    </div>
                    <div className="flex gap-2">
                      <Button variant="outline" size="sm" className="flex-1">
                        <Settings className="h-3.5 w-3.5 mr-1.5" />
                        Configure
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="flex-1"
                        onClick={() => handleTest(connector.id)}
                        disabled={testing === connector.id}
                      >
                        {testing === connector.id ? (
                          <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                        ) : (
                          <Zap className="h-3.5 w-3.5 mr-1.5" />
                        )}
                        Test
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              );
            })
          ) : (
            <Card className="col-span-full">
              <CardContent className="py-8 text-center text-slate-400">
                No connectors configured. Click &quot;Add Connector&quot; to set up integrations.
              </CardContent>
            </Card>
          )}
        </div>
      )}
    </div>
  );
}
