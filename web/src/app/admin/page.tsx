"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Loader2, Shield, Users, ScrollText } from "lucide-react";
import { useApi } from "@/hooks/useApi";
import { toggleKillSwitch } from "@/lib/api";

interface UserEntry {
  id: string;
  email: string;
  name: string;
  role: string;
  created_at: string;
}

interface AuditEntry {
  id: string;
  action: string;
  resource_type: string;
  resource_id: string;
  user_id: string;
  created_at: string;
}

interface ApiResponse<T> {
  data: T;
}

async function fetchUsers(): Promise<{ data: UserEntry[] }> {
  const token = typeof window !== "undefined" ? localStorage.getItem("aegisclaw_token") : "";
  const res = await fetch(
    `${process.env.NEXT_PUBLIC_API_URL || ""}/api/v1/admin/users`,
    { headers: { Authorization: `Bearer ${token}` } }
  );
  if (!res.ok) throw new Error("Failed to load users (admin access required)");
  const body: ApiResponse<UserEntry[]> = await res.json();
  return { data: body.data || [] };
}

async function fetchAuditLog(): Promise<{ data: AuditEntry[] }> {
  const token = typeof window !== "undefined" ? localStorage.getItem("aegisclaw_token") : "";
  const res = await fetch(
    `${process.env.NEXT_PUBLIC_API_URL || ""}/api/v1/admin/audit-log?per_page=20`,
    { headers: { Authorization: `Bearer ${token}` } }
  );
  if (!res.ok) throw new Error("Failed to load audit log");
  const body: ApiResponse<AuditEntry[]> = await res.json();
  return { data: body.data || [] };
}

const roleColor: Record<string, string> = {
  admin: "bg-red-100 text-red-700",
  operator: "bg-blue-100 text-blue-700",
  viewer: "bg-slate-100 text-slate-700",
  approver: "bg-amber-100 text-amber-700",
};

export default function AdminPage() {
  const { data: users, loading: loadingUsers, error: usersError } = useApi<UserEntry[]>(fetchUsers);
  const { data: auditLog, loading: loadingAudit, error: auditError } = useApi<AuditEntry[]>(fetchAuditLog);
  const [killSwitchLoading, setKillSwitchLoading] = useState(false);

  const handleKillSwitch = async (engaged: boolean) => {
    setKillSwitchLoading(true);
    try {
      await toggleKillSwitch(engaged);
      alert(engaged ? "Kill switch ENGAGED — all runs stopped" : "Kill switch disengaged");
    } catch {
      alert("Failed to toggle kill switch");
    } finally {
      setKillSwitchLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Shield className="h-6 w-6 text-slate-400" />
        <h1 className="text-2xl font-bold text-slate-900">Administration</h1>
      </div>

      {/* Kill Switch */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Emergency Kill Switch</CardTitle>
        </CardHeader>
        <CardContent className="flex gap-3">
          <Button
            variant="destructive"
            onClick={() => handleKillSwitch(true)}
            disabled={killSwitchLoading}
          >
            {killSwitchLoading && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
            Engage Kill Switch
          </Button>
          <Button
            variant="outline"
            onClick={() => handleKillSwitch(false)}
            disabled={killSwitchLoading}
          >
            Disengage
          </Button>
        </CardContent>
      </Card>

      {/* Users */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Users className="h-4 w-4" /> Users ({users?.length ?? 0})
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loadingUsers ? (
            <div className="flex items-center gap-2 text-slate-500 py-4 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading...
            </div>
          ) : usersError ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-red-700 text-sm">{usersError}</div>
          ) : users && users.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left">
                    <th className="pb-2 font-medium text-slate-500">Name</th>
                    <th className="pb-2 font-medium text-slate-500">Email</th>
                    <th className="pb-2 font-medium text-slate-500">Role</th>
                    <th className="pb-2 font-medium text-slate-500">Created</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => (
                    <tr key={u.id} className="border-b border-slate-50">
                      <td className="py-2">{u.name}</td>
                      <td className="py-2 text-slate-500">{u.email}</td>
                      <td className="py-2">
                        <Badge className={roleColor[u.role] ?? "bg-slate-100 text-slate-700"}>{u.role}</Badge>
                      </td>
                      <td className="py-2 text-xs text-slate-400">{new Date(u.created_at).toLocaleDateString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="text-center text-slate-400 py-4">No users found</p>
          )}
        </CardContent>
      </Card>

      {/* Audit Log */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <ScrollText className="h-4 w-4" /> Recent Audit Log
          </CardTitle>
        </CardHeader>
        <CardContent>
          {loadingAudit ? (
            <div className="flex items-center gap-2 text-slate-500 py-4 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" /> Loading...
            </div>
          ) : auditError ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-red-700 text-sm">{auditError}</div>
          ) : auditLog && auditLog.length > 0 ? (
            <div className="space-y-2">
              {auditLog.map((entry) => (
                <div key={entry.id} className="flex justify-between items-center text-sm border-b border-slate-50 py-1.5">
                  <div>
                    <Badge variant="outline" className="text-xs mr-2">{entry.action}</Badge>
                    <span className="text-slate-600">{entry.resource_type}</span>
                  </div>
                  <span className="text-xs text-slate-400">{new Date(entry.created_at).toLocaleString()}</span>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-center text-slate-400 py-4">No audit log entries</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
