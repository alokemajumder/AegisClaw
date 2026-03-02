import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

export default function DashboardPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Dashboard</h1>
        <p className="text-sm text-slate-500">
          Security validation posture overview
        </p>
      </div>

      {/* Top metrics */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-slate-500">
              Security Posture Score
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-emerald-600">78.5%</div>
            <p className="text-xs text-slate-500 mt-1">+2.3% from last week</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-slate-500">
              Active Runs
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-blue-600">3</div>
            <p className="text-xs text-slate-500 mt-1">2 Tier 0, 1 Tier 1</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-slate-500">
              Open Findings
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-amber-600">12</div>
            <div className="flex gap-1 mt-1">
              <Badge variant="destructive" className="text-[10px] px-1.5">
                2 Critical
              </Badge>
              <Badge className="text-[10px] px-1.5 bg-orange-100 text-orange-700 hover:bg-orange-100">
                4 High
              </Badge>
              <Badge className="text-[10px] px-1.5 bg-yellow-100 text-yellow-700 hover:bg-yellow-100">
                6 Medium
              </Badge>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-slate-500">
              Coverage Score
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-purple-600">64%</div>
            <p className="text-xs text-slate-500 mt-1">
              128/200 techniques validated
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Second row */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {/* Recent Activity */}
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-base">Recent Activity</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {[
                { time: "2 min ago", text: "Run #347 completed — 15/15 steps passed", color: "#10b981" },
                { time: "12 min ago", text: "Finding: Defender alert latency >5min for T1059.001", color: "#f59e0b" },
                { time: "25 min ago", text: "Connector health check: Sentinel — healthy", color: "#94a3b8" },
                { time: "1 hr ago", text: "Run #346 completed — 12/14 steps passed, 2 findings", color: "#f59e0b" },
                { time: "2 hrs ago", text: "Engagement 'Quarterly AD Validation' activated", color: "#94a3b8" },
              ].map((item, i) => (
                <div key={i} className="flex items-start gap-3 text-sm border-l-2 pl-3 py-1" style={{ borderColor: item.color }}>
                  <span className="text-slate-400 text-xs whitespace-nowrap w-16">{item.time}</span>
                  <span className="text-slate-700">{item.text}</span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Connector Health */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Connector Health</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {[
                { name: "Sentinel", status: "healthy", category: "SIEM" },
                { name: "Defender", status: "healthy", category: "EDR" },
                { name: "Entra ID", status: "healthy", category: "Identity" },
                { name: "ServiceNow", status: "degraded", category: "ITSM" },
                { name: "Teams", status: "healthy", category: "Notifications" },
                { name: "Slack", status: "healthy", category: "Notifications" },
              ].map((c, i) => (
                <div key={i} className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium text-slate-700">{c.name}</p>
                    <p className="text-xs text-slate-400">{c.category}</p>
                  </div>
                  <Badge className={c.status === "healthy" ? "bg-emerald-100 text-emerald-700 hover:bg-emerald-100" : "bg-amber-100 text-amber-700 hover:bg-amber-100"}>
                    {c.status}
                  </Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
