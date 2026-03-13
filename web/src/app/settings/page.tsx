"use client";

import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Settings, Shield, Plug, Bot, ExternalLink, Info, Loader2, User } from "lucide-react";
import Link from "next/link";
import { getMe } from "@/lib/api";
import type { User as UserType } from "@/lib/types";

export default function SettingsPage() {
  const [user, setUser] = useState<UserType | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getMe()
      .then(setUser)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Platform Configuration</h1>
        <p className="text-sm text-slate-500">
          View AegisClaw platform settings. System-level settings are managed via environment variables for security.
        </p>
      </div>

      <div className="flex items-start gap-2 rounded-md bg-blue-50 border border-blue-200 px-4 py-3 text-sm text-blue-700">
        <Info className="h-4 w-4 mt-0.5 shrink-0" />
        <span>
          Platform configuration is managed through environment variables and deployment manifests.
          Changes require a service restart to take effect. See{" "}
          <code className="bg-blue-100 px-1 rounded text-xs font-mono">deploy/docker-compose.yml</code>{" "}
          for all configurable options.
        </span>
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
              <CardTitle className="text-base">Current User</CardTitle>
            </CardHeader>
            <CardContent>
              {loading ? (
                <div className="flex items-center gap-2 text-slate-500">
                  <Loader2 className="h-4 w-4 animate-spin" /> Loading...
                </div>
              ) : user ? (
                <div className="space-y-3 text-sm max-w-lg">
                  <div className="flex justify-between">
                    <span className="text-slate-500">Name</span>
                    <span className="font-medium">{user.full_name}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-500">Email</span>
                    <span>{user.email}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-500">Role</span>
                    <Badge variant="outline">{user.role}</Badge>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-slate-500">Organization</span>
                    <span className="font-mono text-xs">{user.org_id.slice(0, 8)}...</span>
                  </div>
                </div>
              ) : (
                <p className="text-slate-400 text-sm">Unable to load user info.</p>
              )}
            </CardContent>
          </Card>

          {user?.role === "admin" && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base flex items-center gap-2">
                  <User className="h-4 w-4" />
                  Administration
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-slate-500 mb-3">
                  Manage users, roles, and organization settings.
                </p>
                <Link
                  href="/admin"
                  className="inline-flex items-center gap-1.5 text-sm text-blue-600 hover:underline"
                >
                  Open Admin Panel <ExternalLink className="h-3.5 w-3.5" />
                </Link>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        {/* Policies Tab */}
        <TabsContent value="policies" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Policy Tiers</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 max-w-lg">
              <p className="text-sm text-slate-500">
                AegisClaw uses a tiered policy model to control the scope and risk of security validations.
                Policy settings are configured per-engagement.
              </p>
              <div className="space-y-2">
                {[
                  { tier: 0, label: "Passive Reconnaissance", desc: "Read-only, no impact. DNS lookups, port scans, banner grabs." },
                  { tier: 1, label: "Active Enumeration", desc: "Low-risk probes. Service fingerprinting, credential spraying (rate-limited)." },
                  { tier: 2, label: "Exploitation (Guarded)", desc: "Safe exploit attempts with known rollback. Requires approval above this tier." },
                  { tier: 3, label: "Controlled Impact", desc: "Write operations, persistence testing. Sandboxed execution with full evidence capture." },
                ].map(({ tier, label, desc }) => (
                  <div key={tier} className="border rounded-lg p-3">
                    <div className="flex items-center gap-2 mb-1">
                      <Badge variant="outline" className="font-mono text-xs">Tier {tier}</Badge>
                      <span className="text-sm font-medium">{label}</span>
                    </div>
                    <p className="text-xs text-slate-400">{desc}</p>
                  </div>
                ))}
              </div>
              <div className="pt-2">
                <Link
                  href="/engagements"
                  className="inline-flex items-center gap-1.5 text-sm text-blue-600 hover:underline"
                >
                  Manage engagement policies <ExternalLink className="h-3.5 w-3.5" />
                </Link>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Connectors Tab */}
        <TabsContent value="connectors" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Connector Management</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 max-w-lg">
              <p className="text-sm text-slate-500">
                Connectors integrate AegisClaw with external security tools and notification services.
                Each connector instance is configured with its own credentials and settings.
              </p>
              <div className="space-y-2 text-sm">
                <div className="border rounded-lg p-3">
                  <span className="font-medium">Supported Connectors</span>
                  <div className="flex flex-wrap gap-1.5 mt-2">
                    {["Microsoft Sentinel", "Microsoft Defender", "ServiceNow", "Microsoft Teams", "Slack"].map(name => (
                      <Badge key={name} variant="outline" className="text-xs">{name}</Badge>
                    ))}
                  </div>
                </div>
              </div>
              <div className="pt-2">
                <Link
                  href="/connectors"
                  className="inline-flex items-center gap-1.5 text-sm text-blue-600 hover:underline"
                >
                  Manage connectors <ExternalLink className="h-3.5 w-3.5" />
                </Link>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* Ollama Tab */}
        <TabsContent value="ollama" className="space-y-4 mt-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Ollama LLM Configuration</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 max-w-lg">
              <p className="text-sm text-slate-500">
                AegisClaw uses Ollama for local LLM inference. All AI reasoning stays on-premises.
                Ollama is optional — agents fall back to deterministic logic when unavailable.
              </p>
              <div className="space-y-2">
                {[
                  { label: "Endpoint", envVar: "OLLAMA_URL", defaultVal: "http://ollama:11434" },
                  { label: "Default Model", envVar: "OLLAMA_MODEL", defaultVal: "llama3" },
                  { label: "Request Timeout", envVar: "OLLAMA_TIMEOUT", defaultVal: "120s" },
                ].map(({ label, envVar, defaultVal }) => (
                  <div key={envVar} className="border rounded-lg p-3 flex justify-between items-center">
                    <div>
                      <span className="text-sm font-medium">{label}</span>
                      <p className="text-xs text-slate-400 mt-0.5">
                        Env: <code className="bg-slate-100 px-1 rounded font-mono">{envVar}</code>
                      </p>
                    </div>
                    <Badge variant="outline" className="font-mono text-xs">{defaultVal}</Badge>
                  </div>
                ))}
              </div>
              <div className="flex items-start gap-2 rounded-md bg-amber-50 border border-amber-200 px-3 py-2 text-xs text-amber-700">
                <Info className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                <span>
                  To change Ollama settings, update the environment variables in your deployment configuration
                  and restart the ollama-bridge service.
                </span>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
