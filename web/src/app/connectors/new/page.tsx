"use client";

import { useState } from "react";
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
} from "lucide-react";

const connectorTypes = [
  {
    name: "Microsoft Sentinel",
    category: "SIEM",
    icon: Shield,
    description: "Cloud-native SIEM for intelligent security analytics.",
  },
  {
    name: "Microsoft Defender",
    category: "EDR",
    icon: MonitorSmartphone,
    description: "Endpoint detection and response platform.",
  },
  {
    name: "Entra ID",
    category: "Identity",
    icon: UserCheck,
    description: "Identity and access management service.",
  },
  {
    name: "ServiceNow",
    category: "ITSM",
    icon: Ticket,
    description: "IT service management and ticketing.",
  },
  {
    name: "Microsoft Teams",
    category: "Notifications",
    icon: MessageSquare,
    description: "Team collaboration and notifications.",
  },
  {
    name: "Slack",
    category: "Notifications",
    icon: Cloud,
    description: "Channel-based messaging and alerts.",
  },
];

const categoryColor: Record<string, string> = {
  SIEM: "bg-blue-100 text-blue-700 hover:bg-blue-100",
  EDR: "bg-purple-100 text-purple-700 hover:bg-purple-100",
  ITSM: "bg-amber-100 text-amber-700 hover:bg-amber-100",
  Identity: "bg-emerald-100 text-emerald-700 hover:bg-emerald-100",
  Notifications: "bg-indigo-100 text-indigo-700 hover:bg-indigo-100",
};

const steps = ["Select Type", "Configure", "Test Connection"];

export default function NewConnectorPage() {
  const [currentStep, setCurrentStep] = useState(0);
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [testStatus, setTestStatus] = useState<
    "idle" | "testing" | "success" | "failed"
  >("idle");

  const handleTest = () => {
    setTestStatus("testing");
    setTimeout(() => setTestStatus("success"), 1500);
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
              {i < currentStep ? (
                <Check className="h-4 w-4" />
              ) : (
                i + 1
              )}
            </div>
            <span
              className={`text-sm ${
                i <= currentStep
                  ? "text-slate-900 font-medium"
                  : "text-slate-400"
              }`}
            >
              {step}
            </span>
            {i < steps.length - 1 && (
              <div className="w-12 h-px bg-slate-200 mx-1" />
            )}
          </div>
        ))}
      </div>

      {/* Step 1: Select Type */}
      {currentStep === 0 && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {connectorTypes.map((type, i) => {
            const IconComponent = type.icon;
            const isSelected = selectedType === type.name;
            return (
              <Card
                key={i}
                className={`cursor-pointer transition-all ${
                  isSelected
                    ? "ring-2 ring-blue-600 border-blue-600"
                    : "hover:border-slate-300"
                }`}
                onClick={() => setSelectedType(type.name)}
              >
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-3">
                    <div className="p-2 rounded-lg bg-slate-100">
                      <IconComponent className="h-5 w-5 text-slate-600" />
                    </div>
                    <div>
                      <CardTitle className="text-base">{type.name}</CardTitle>
                      <Badge
                        className={`mt-1 text-[10px] ${categoryColor[type.category]}`}
                      >
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
      )}

      {/* Step 2: Configure */}
      {currentStep === 1 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              Configure {selectedType}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4 max-w-lg">
            <div className="space-y-2">
              <Label htmlFor="name">Connector Name</Label>
              <Input
                id="name"
                placeholder={`e.g., ${selectedType} Production`}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                placeholder="Brief description of this connector instance"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="baseUrl">Base URL</Label>
              <Input
                id="baseUrl"
                placeholder="https://api.example.com/v1"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="apiKey">API Key / Client Secret</Label>
              <Input id="apiKey" type="password" placeholder="Enter secret" />
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
              Verify that AegisClaw can communicate with {selectedType} using
              the provided configuration.
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
        {currentStep < steps.length - 1 ? (
          <Button
            onClick={() => setCurrentStep(currentStep + 1)}
            disabled={currentStep === 0 && !selectedType}
          >
            Next
            <ArrowRight className="h-4 w-4 ml-2" />
          </Button>
        ) : (
          <Button disabled={testStatus !== "success"}>
            <Check className="h-4 w-4 mr-2" />
            Save Connector
          </Button>
        )}
      </div>
    </div>
  );
}
