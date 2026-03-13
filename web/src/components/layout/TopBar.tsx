"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Bell, OctagonX, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { getMe, logout, toggleKillSwitch, getDashboardHealth } from "@/lib/api";
import type { User } from "@/lib/types";

export default function TopBar() {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(null);
  const [killSwitchEngaged, setKillSwitchEngaged] = useState(false);
  const [toggling, setToggling] = useState(false);

  useEffect(() => {
    getMe()
      .then(setUser)
      .catch(() => {});

    getDashboardHealth()
      .then((resp) => {
        setKillSwitchEngaged(resp.data?.kill_switch_engaged ?? false);
      })
      .catch(() => {});
  }, []);

  const handleToggleKillSwitch = async () => {
    setToggling(true);
    try {
      const resp = await toggleKillSwitch(!killSwitchEngaged);
      setKillSwitchEngaged(resp.data?.engaged ?? !killSwitchEngaged);
    } catch {
      // Error handled
    } finally {
      setToggling(false);
    }
  };

  const handleLogout = () => {
    logout();
    router.push("/login");
  };

  const initials = user
    ? user.name
        .split(" ")
        .map((n) => n[0])
        .join("")
        .toUpperCase()
        .slice(0, 2)
    : "??";

  return (
    <header className="h-14 border-b border-slate-200 bg-white flex items-center justify-between px-6">
      {/* Spacer */}
      <div className="flex-1" />

      {/* Right side */}
      <div className="flex items-center gap-3">
        {/* Kill Switch */}
        <Button
          variant={killSwitchEngaged ? "default" : "destructive"}
          size="sm"
          className={`gap-1.5 h-8 ${killSwitchEngaged ? "bg-red-700 hover:bg-red-800" : ""}`}
          onClick={handleToggleKillSwitch}
          disabled={toggling}
        >
          {toggling ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <OctagonX className="h-3.5 w-3.5" />
          )}
          {killSwitchEngaged ? "Kill Switch ON" : "Kill Switch"}
        </Button>

        {/* Notifications */}
        <Button variant="ghost" size="sm" className="relative h-8 w-8 p-0" aria-label="View notifications">
          <Bell className="h-4 w-4" />
        </Button>

        {/* User menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className="gap-2 h-8">
              <Avatar className="h-6 w-6">
                <AvatarFallback className="bg-emerald-100 text-emerald-700 text-xs">
                  {initials}
                </AvatarFallback>
              </Avatar>
              <span className="text-sm text-slate-700">
                {user?.name ?? "Loading..."}
              </span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem>Profile</DropdownMenuItem>
            <DropdownMenuItem>Preferences</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem className="text-red-600" onClick={handleLogout}>
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
