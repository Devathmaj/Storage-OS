import { Search, Moon, Sun, User, Cloud, LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useTheme } from "next-themes";
import { useNavigate } from "react-router-dom";
import { useAuth } from "@/contexts/AuthContext";

interface NavbarProps {
  breadcrumbs: string[];
  onSearch: (query: string) => void;
  onBreadcrumbClick?: (index: number) => void;
}

export const Navbar = ({ breadcrumbs, onSearch, onBreadcrumbClick }: NavbarProps) => {
  const { theme, setTheme } = useTheme();
  const navigate = useNavigate();
  const { logout, user } = useAuth();

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  return (
    <nav className="h-16 border-b bg-card/50 glass-effect sticky top-0 z-40">
      <div className="h-full px-6 flex items-center justify-between gap-4">
        <div className="flex items-center gap-4 flex-1">
          <div className="flex items-center gap-2">
            <Cloud className="h-6 w-6 text-primary" />
            <span className="font-semibold text-lg">Cloudlet</span>
          </div>
          
          <div className="hidden md:flex items-center gap-1 text-sm text-muted-foreground">
            {breadcrumbs.map((crumb, index) => (
              <div key={index} className="flex items-center gap-1">
                {index > 0 && <span>/</span>}
                <button
                  type="button"
                  className="hover:text-foreground transition-colors px-1"
                  onClick={() => onBreadcrumbClick?.(index)}
                >
                  {crumb}
                </button>
              </div>
            ))}
          </div>
        </div>

        <div className="flex items-center gap-3">
          <div className="relative hidden md:block">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder="Search files..."
              className="pl-9 w-64 bg-background/60"
              onChange={(e) => onSearch(e.target.value)}
            />
          </div>

          <Button
            variant="ghost"
            size="icon"
            onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
          >
            {theme === "dark" ? (
              <Sun className="h-5 w-5" />
            ) : (
              <Moon className="h-5 w-5" />
            )}
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="icon">
                <User className="h-5 w-5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <div className="px-2 py-1.5 text-sm font-medium">
                {user?.username}
              </div>
              <div className="px-2 py-1 text-xs text-muted-foreground">
                {user?.email}
              </div>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => navigate("/profile")}>
                <User className="mr-2 h-4 w-4" />
                Profile
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleLogout} className="text-red-600 dark:text-red-400">
                <LogOut className="mr-2 h-4 w-4" />
                Logout
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </nav>
  );
};
