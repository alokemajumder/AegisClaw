"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  ArrowLeft,
  ArrowRight,
  Shield,
  MonitorSmartphone,
  UserCheck,
  Ticket,
  MessageSquare,
  Cloud,
  Check,
  Zap,
  Loader2,
} from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { listConnectorRegistry, createConnectorInstance, testConnector } from "@/lib/api";
import type { ConnectorRegistry } from "@/lib/types";

const categoryColor: Record<string, string> = {
  SIEM: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  EDR: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  ITSM: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  Identity: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Notifications: "bg-indigo-100 text-indigo-700 hover:bg-indigo-100",
};

const iconMap: Record<string, typeof Shield> = {
  SIEM: Shield,
  EDR: MonitorSmartphone,
  Identity: UserCheck,
  ITSM: Ticket,
  Notifications: MessageSquare,
  Cloud: Cloud,
};

const steps = ["Select Type", "Configure", "Test Connection"];

export default function NewConnectorPage() {
  const router = useRouter();
  const { data: registry, loading, error } = useApi<ConnectorRegistry[]>(() => listConnectorRegistry());

  const [currentStep, setCurrentStep] = useState(0);
  const [selectedType, setSelectedType] = useState<ConnectorRegistry | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [config, setConfig] = useState<Record<string, string>>({});
  const [createdId, setCreatedId] = useState<string | null>(null);
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "success" | "failed">("idle");
  const [saving, setSaving] = useState(false);

  const handleCreateAndTest = async () => {
    setSaving(true);
    try {
      const resp = await createConnectorInstance({
        registry_id: selectedType!.id,
        name,
        description,
        config,
        enabled: true,
      });
      setCreatedId(resp.data.id);
      setCurrentStep(2);
    } catch {
      // Error handled
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    if (!createdId) return;
    setTestStatus("testing");
    try {
      await testConnector(createdId);
      setTestStatus("success");
    } catch {
      setTestStatus("failed");
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="sm" asChild>
          <a href="/connectors">
            <ArrowLeft className="h-4 w-4 mr-1" />
            Back
          </a>
        </Button>
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Add Connector</h1>
          <p className="text-sm text-slate-500">
            Set up a new security platform integration
          </p>
        </div>
      </div>

      {/* Step Indicator */}
      <div className="flex items-center gap-2">
        {steps.map((step, i) => (
          <div key={i} className="flex items-center gap-2">
            <div
              className={`flex items-center justify-center h-8 w-8 rounded-full text-sm font-medium ${
                i <= currentStep
                  ? "bg-blue-600 text-white"
                  : "bg-slate-200 text-slate-500"
              }`}
            >
              {i < currentStep ? <Check className="h-4 w-4" /> : i + 1}
            </div>
            <span className={`text-sm ${i <= currentStep ? "text-slate-900 font-medium" : "text-slate-400"}`}>
              {step}
            </span>
            {i < steps.length - 1 && <div className="w-12 h-px bg-slate-200 mx-1" />}
          </div>
        ))}
      </div>

      {/* Step 1: Select Type */}
      {currentStep === 0 && (
        loading ? (
          <div className="flex items-center gap-2 text-slate-500">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading connector types...
          </div>
        ) : error ? (
          <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
            Failed to load data: {error}
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {(registry ?? []).map((type) => {
              const IconComponent = iconMap[type.category] ?? Shield;
              const isSelected = selectedType?.id === type.id;
              return (
                <Card
                  key={type.id}
                  className={`cursor-pointer transition-all ${isSelected ? "ring-2 ring-blue-600 border-blue-600" : "hover:border-slate-300"}`}
                  onClick={() => setSelectedType(type)}
                >
                  <CardHeader className="pb-2">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-slate-100">
                        <IconComponent className="h-5 w-5 text-slate-600" />
                      </div>
                      <div>
                        <CardTitle className="text-base">{type.name}</CardTitle>
                        <Badge className={`mt-1 text-[10px] ${categoryColor[type.category] ?? "bg-slate-100 text-slate-700"}`}>
                          {type.category}
                        </Badge>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <p className="text-sm text-slate-500">{type.description}</p>
                  </CardContent>
                </Card>
              );
            })}
          </div>
        )
      )}

      {/* Step 2: Configure */}
      {currentStep === 1 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              Configure {selectedType?.name}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 max-w-lg">
            <div className="space-y-2">
              <Label htmlFor="name">Connector Name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder={`e.g., ${selectedType?.name ?? "Connector"} Production`} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input id="description" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Brief description of this connector instance" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="baseUrl">Base URL</Label>
              <Input id="baseUrl" value={config.base_url ?? ""} onChange={(e) => setConfig({ ...config, base_url: e.target.value })} placeholder="https://api.example.com/v1" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="apiKey">API Key / Client Secret</Label>
              <Input id="apiKey" type="password" value={config.api_key ?? ""} onChange={(e) => setConfig({ ...config, api_key: e.target.value })} placeholder="Enter secret" />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Step 3: Test Connection */}
      {currentStep === 2 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Test Connection</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-slate-500">
              Verify that AegisClaw can communicate with {selectedType?.name} using the provided configuration.
            </p>
            <Button onClick={handleTest} disabled={testStatus === "testing"}>
              <Zap className="h-4 w-4 mr-2" />
              {testStatus === "testing" ? "Testing..." : "Run Connection Test"}
            </Button>
            {testStatus === "success" && (
              <div className="flex items-center gap-2 text-emerald-600 text-sm">
                <Check className="h-4 w-4" />
                Connection successful. Connector is ready.
              </div>
            )}
            {testStatus === "failed" && (
              <div className="text-red-600 text-sm">
                Connection failed. Please check your configuration.
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Navigation */}
      <div className="flex justify-between">
        <Button
          variant="outline"
          onClick={() => setCurrentStep(Math.max(0, currentStep - 1))}
          disabled={currentStep === 0}
        >
          <ArrowLeft className="h-4 w-4 mr-2" />
          Previous
        </Button>
        {currentStep === 0 && (
          <Button onClick={() => setCurrentStep(1)} disabled={!selectedType}>
            Next
            <ArrowRight className="h-4 w-4 ml-2" />
          </Button>
        )}
        {currentStep === 1 && (
          <Button onClick={handleCreateAndTest} disabled={saving || !name}>
            {saving ? <Loader2 className="h-4 w-4 mr-2 animate-spin" /> : null}
            Create & Test
            <ArrowRight className="h-4 w-4 ml-2" />
          </Button>
        )}
        {currentStep === 2 && (
          <Button onClick={() => router.push("/connectors")} disabled={testStatus !== "success"}>
            <Check className="h-4 w-4 mr-2" />
            Done
          </Button>
        )}
      </div>
    </div>
  );
}
