"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Server,
  Target,
  Play,
  AlertTriangle,
  Plug,
  CheckCircle,
  FileText,
  Grid3X3,
  Settings,
  Shield,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { useState } from "react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

const navItems = [
  { label: "Dashboard", href: "/", icon: LayoutDashboard },
  { label: "Assets", href: "/assets", icon: Server },
  { label: "Engagements", href: "/engagements", icon: Target },
  { label: "Runs", href: "/runs", icon: Play },
  { label: "Findings", href: "/findings", icon: AlertTriangle },
  { label: "Connectors", href: "/connectors", icon: Plug },
  { label: "Approvals", href: "/approvals", icon: CheckCircle },
  { label: "Reports", href: "/reports", icon: FileText },
  { label: "Coverage", href: "/coverage", icon: Grid3X3 },
];

const bottomItems = [
  { label: "Settings", href: "/settings", icon: Settings },
  { label: "Admin", href: "/admin", icon: Shield },
];

export default function Sidebar() {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(false);

  return (
    <aside
      className={cn(
        "flex flex-col bg-slate-900 text-slate-300 border-r border-slate-800 transition-all duration-200",
        collapsed ? "w-16" : "w-60"
      )}
    >
      {/* Logo */}
      <div className="flex items-center h-14 px-4 border-b border-slate-800">
        <Shield className="h-6 w-6 text-emerald-400 shrink-0" />
        {!collapsed && (
          <span className="ml-2 text-lg font-bold text-white">AegisClaw</span>
        )}
      </div>

      {/* Main nav */}
      <nav className="flex-1 py-4 space-y-1 overflow-y-auto">
        {navItems.map((item) => {
          const isActive =
            pathname === item.href ||
            (item.href !== "/" && pathname.startsWith(item.href));
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-3 px-4 py-2 text-sm transition-colors",
                isActive
                  ? "bg-slate-800 text-emerald-400 border-r-2 border-emerald-400"
                  : "hover:bg-slate-800/50 hover:text-white"
              )}
            >
              <item.icon className="h-4 w-4 shrink-0" />
              {!collapsed && <span>{item.label}</span>}
            </Link>
          );
        })}
      </nav>

      {/* Bottom nav */}
      <div className="pb-2">
        <Separator className="bg-slate-800 mb-2" />
        {bottomItems.map((item) => {
          const isActive = pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-3 px-4 py-2 text-sm transition-colors",
                isActive
                  ? "bg-slate-800 text-emerald-400"
                  : "hover:bg-slate-800/50 hover:text-white"
              )}
            >
              <item.icon className="h-4 w-4 shrink-0" />
              {!collapsed && <span>{item.label}</span>}
            </Link>
          );
        })}

        {/* Collapse toggle */}
        <Button
          variant="ghost"
          size="sm"
          className="w-full mt-2 text-slate-500 hover:text-white"
          onClick={() => setCollapsed(!collapsed)}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          {collapsed ? (
            <ChevronRight className="h-4 w-4" />
          ) : (
            <ChevronLeft className="h-4 w-4" />
          )}
        </Button>
      </div>
    </aside>
  );
}
