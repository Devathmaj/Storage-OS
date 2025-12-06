import { Home, Clock, Star, Trash2, Settings, Users } from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { Link, useLocation } from "react-router-dom";
import { useAuth } from "@/contexts/AuthContext";

const menuItems = [
  { title: "My Drive", icon: Home, path: "/", adminOnly: false },
  { title: "Recent", icon: Clock, path: "/recent", adminOnly: false },
  { title: "Favorites", icon: Star, path: "/favorites", adminOnly: false },
  { title: "Trash", icon: Trash2, path: "/trash", adminOnly: false },
  { title: "Settings", icon: Settings, path: "/settings", adminOnly: false },
  { title: "User Management", icon: Users, path: "/user-management", adminOnly: true },
];

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return "0 Bytes";
  const k = 1024;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
};

export const AppSidebar = () => {
  const { state } = useSidebar();
  const isCollapsed = state === "collapsed";
  const location = useLocation();
  const current = location.pathname;
  const { user } = useAuth();

  const usedStorage = user?.used_storage || 0;
  const maxStorage = user?.max_storage || 0;
  const storagePercentage = maxStorage > 0 ? Math.round((usedStorage / maxStorage) * 100) : 0;
  const isAdmin = user?.role === "admin";

  const visibleMenuItems = menuItems.filter(item => !item.adminOnly || isAdmin);

  return (
    <Sidebar className={isCollapsed ? "w-16" : "w-60"} collapsible="icon">
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel className={isCollapsed ? "hidden" : ""}>
            Storage
          </SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {visibleMenuItems.map((item) => {
                const isActive = current === item.path;
                return (
                  <SidebarMenuItem key={item.title}>
                    <SidebarMenuButton
                      asChild
                      className={isActive ? "bg-sidebar-accent text-sidebar-primary font-medium" : ""}
                    >
                      <Link to={item.path} className="w-full flex items-center gap-3 py-2">
                        <item.icon className="h-4 w-4" />
                        {!isCollapsed && <span>{item.title}</span>}
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                );
              })}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {!isCollapsed && user && maxStorage > 0 && (
          <div className="px-4 py-6 mt-auto border-t">
            <div className="text-xs text-muted-foreground space-y-1">
              <div className="flex justify-between">
                <span>Storage used</span>
                <span>{storagePercentage}%</span>
              </div>
              <div className="h-1.5 bg-muted rounded-full overflow-hidden">
                <div 
                  className="h-full bg-primary rounded-full" 
                  style={{ width: `${Math.min(storagePercentage, 100)}%` }}
                />
              </div>
              <div className="text-center pt-1">
                {formatBytes(usedStorage)} of {formatBytes(maxStorage)} used
              </div>
            </div>
          </div>
        )}
        {!isCollapsed && user && maxStorage === 0 && (
          <div className="px-4 py-6 mt-auto border-t">
            <div className="text-xs text-muted-foreground space-y-1">
              <div className="text-center">
                <span className="font-medium">Unlimited Storage</span>
              </div>
              <div className="text-center pt-1">
                {formatBytes(usedStorage)} used
              </div>
            </div>
          </div>
        )}
      </SidebarContent>
    </Sidebar>
  );
};
