"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Settings, Shield, Plug, Bot, Save } from "lucide-react";

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Settings</h1>
        <p className="text-sm text-slate-500">
          Configure AegisClaw platform settings
        </p>
      </div>

      <Tabs defaultValue="general">
        <TabsList>
          <TabsTrigger value="general" className="gap-1.5">
            <Settings className="h-3.5 w-3.5" />
            General
          </TabsTrigger>
          <TabsTrigger value="policies" className="gap-1.5">
            <Shield className="h-3.5 w-3.5" />
            Policies
          </TabsTrigger>
          <TabsTrigger value="connectors" className="gap-1.5">
            <Plug className="h-3.5 w-3.5" />
            Connectors
          </TabsTrigger>
          <TabsTrigger value="ollama" className="gap-1.5">
            <Bot className="h-3.5 w-3.5" />
            Ollama
          </TabsTrigger>
        </TabsList>

        {/* General Tab */}
        <TabsContent value="general" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Organization</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 max-w-lg">
              <div className="space-y-2">
                <Label htmlFor="orgName">Organization Name</Label>
                <Input id="orgName" defaultValue="Contoso Security" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="policyPack">Default Policy Pack</Label>
                <Select defaultValue="standard">
                  <SelectTrigger id="policyPack">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="standard">Standard</SelectItem>
                    <SelectItem value="strict">Strict</SelectItem>
                    <SelectItem value="permissive">Permissive</SelectItem>
                    <SelectItem value="custom">Custom</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="rateLimit">Global Rate Limit (req/min)</Label>
                <Input
                  id="rateLimit"
                  type="number"
                  defaultValue="60"
                  className="max-w-[200px]"
                />
                <p className="text-xs text-slate-400">
                  Maximum API requests per minute across all connectors.
                </p>
              </div>
              <Button>
                <Save className="h-4 w-4 mr-2" />
                Save Changes
              </Button>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Policies Tab */}
        <TabsContent value="policies" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Policy Configuration</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4 max-w-lg">
                <div className="space-y-2">
                  <Label htmlFor="approvalThreshold">
                    Approval Required Above Tier
                  </Label>
                  <Select defaultValue="2">
                    <SelectTrigger id="approvalThreshold">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="0">Tier 0 (all actions)</SelectItem>
                      <SelectItem value="1">Tier 1</SelectItem>
                      <SelectItem value="2">Tier 2</SelectItem>
                      <SelectItem value="3">Tier 3</SelectItem>
                    </SelectContent>
                  </Select>
                  <p className="text-xs text-slate-400">
                    Actions at or above this tier will require manual approval
                    before execution.
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="maxConcurrentRuns">
                    Max Concurrent Runs
                  </Label>
                  <Input
                    id="maxConcurrentRuns"
                    type="number"
                    defaultValue="5"
                    className="max-w-[200px]"
                  />
                </div>
                <Button>
                  <Save className="h-4 w-4 mr-2" />
                  Save Policies
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Connectors Tab */}
        <TabsContent value="connectors" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">
                Connector Global Settings
              </CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4 max-w-lg">
                <div className="space-y-2">
                  <Label htmlFor="healthCheckInterval">
                    Health Check Interval (seconds)
                  </Label>
                  <Input
                    id="healthCheckInterval"
                    type="number"
                    defaultValue="300"
                    className="max-w-[200px]"
                  />
                  <p className="text-xs text-slate-400">
                    How often to poll connector health status.
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="retryAttempts">
                    Retry Attempts on Failure
                  </Label>
                  <Input
                    id="retryAttempts"
                    type="number"
                    defaultValue="3"
                    className="max-w-[200px]"
                  />
                </div>
                <Button>
                  <Save className="h-4 w-4 mr-2" />
                  Save Settings
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Ollama Tab */}
        <TabsContent value="ollama" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Ollama Configuration</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4 max-w-lg">
                <div className="space-y-2">
                  <Label htmlFor="ollamaEndpoint">Ollama Endpoint</Label>
                  <Input
                    id="ollamaEndpoint"
                    defaultValue="http://localhost:11434"
                  />
                  <p className="text-xs text-slate-400">
                    The base URL of your Ollama instance for LLM-driven
                    analysis.
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ollamaModel">Default Model</Label>
                  <Select defaultValue="llama3">
                    <SelectTrigger id="ollamaModel">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="llama3">llama3</SelectItem>
                      <SelectItem value="mistral">mistral</SelectItem>
                      <SelectItem value="codellama">codellama</SelectItem>
                      <SelectItem value="phi3">phi3</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ollamaTimeout">
                    Request Timeout (seconds)
                  </Label>
                  <Input
                    id="ollamaTimeout"
                    type="number"
                    defaultValue="120"
                    className="max-w-[200px]"
                  />
                </div>
                <Button>
                  <Save className="h-4 w-4 mr-2" />
                  Save Ollama Settings
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
