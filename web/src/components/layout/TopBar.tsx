"use client";

import { Bell, OctagonX, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export default function TopBar() {
  return (
    <header className="h-14 border-b border-slate-200 bg-white flex items-center justify-between px-6">
      {/* Search */}
      <div className="flex items-center gap-2 flex-1 max-w-md">
        <Search className="h-4 w-4 text-slate-400" />
        <Input
          placeholder="Search assets, findings, runs..."
          className="border-0 bg-slate-50 focus-visible:ring-0 h-8"
        />
      </div>

      {/* Right side */}
      <div className="flex items-center gap-3">
        {/* Kill Switch */}
        <Button
          variant="destructive"
          size="sm"
          className="gap-1.5 h-8"
        >
          <OctagonX className="h-3.5 w-3.5" />
          Kill Switch
        </Button>

        {/* Notifications */}
        <Button variant="ghost" size="sm" className="relative h-8 w-8 p-0">
          <Bell className="h-4 w-4" />
          <Badge className="absolute -top-1 -right-1 h-4 w-4 p-0 flex items-center justify-center text-[10px] bg-red-500">
            3
          </Badge>
        </Button>

        {/* User menu */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" className="gap-2 h-8">
              <Avatar className="h-6 w-6">
                <AvatarFallback className="bg-emerald-100 text-emerald-700 text-xs">
                  AU
                </AvatarFallback>
              </Avatar>
              <span className="text-sm text-slate-700">Admin</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem>Profile</DropdownMenuItem>
            <DropdownMenuItem>Preferences</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem className="text-red-600">Log out</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
