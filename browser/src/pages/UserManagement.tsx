import { useState, useEffect } from "react";
import { ThemeProvider } from "next-themes";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Navbar } from "@/components/Navbar";
import { useAuth } from "@/contexts/AuthContext";
import { useToast } from "@/hooks/use-toast";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { UserPlus, Users, Mail, Key, HardDrive } from "lucide-react";

const API_BASE = "http://localhost:8081";

interface User {
  id: string;
  username: string;
  email: string;
  role: string;
  used_storage: number;
  max_storage: number;
  status: string;
  created_at: string;
}

export default function UserManagement() {
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [formData, setFormData] = useState({
    username: "",
    email: "",
    password: "",
    max_storage_gb: 10,
  });
  const { token } = useAuth();
  const { toast } = useToast();

  useEffect(() => {
    if (token) {
      fetchUsers();
    }
  }, [token]);

  const fetchUsers = async () => {
    try {
      const response = await fetch(`${API_BASE}/v1/admin/users`, {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error("Failed to fetch users");
      }

      const data = await response.json();
      setUsers(data.users || []);
    } catch (error) {
      console.error("Error fetching users:", error);
      toast({
        title: "Error",
        description: "Failed to load users",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleCreateUser = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!formData.username || !formData.email || !formData.password) {
      toast({
        title: "Error",
        description: "All fields are required",
        variant: "destructive",
      });
      return;
    }

    try {
      const response = await fetch(`${API_BASE}/v1/admin/users`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify(formData),
      });

      if (!response.ok) {
        const error = await response.text();
        throw new Error(error || "Failed to create user");
      }

      toast({
        title: "Success",
        description: "User created successfully",
      });

      setDialogOpen(false);
      setFormData({ username: "", email: "", password: "", max_storage_gb: 10 });
      fetchUsers();
    } catch (error: any) {
      console.error("Error creating user:", error);
      toast({
        title: "Error",
        description: error.message || "Failed to create user",
        variant: "destructive",
      });
    }
  };

  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  const getStoragePercentage = (used: number, max: number): number => {
    return max > 0 ? Math.round((used / max) * 100) : 0;
  };

  return (
    <ThemeProvider attribute="class" defaultTheme="light">
      <SidebarProvider>
        <div className="flex min-h-screen w-full">
          <AppSidebar />

          <div className="flex-1 flex flex-col">
            <div className="border-b bg-background/60 px-4 py-2">
              <SidebarTrigger />
            </div>

            <Navbar breadcrumbs={["Admin", "User Management"]} onSearch={() => {}} />

            <main className="p-6">
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h1 className="text-2xl font-semibold mb-2">User Management</h1>
                  <p className="text-muted-foreground">Manage user accounts and storage quotas</p>
                </div>

                <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
                  <DialogTrigger asChild>
                    <Button>
                      <UserPlus className="mr-2 h-4 w-4" />
                      Create User
                    </Button>
                  </DialogTrigger>
                  <DialogContent>
                    <DialogHeader>
                      <DialogTitle>Create New User</DialogTitle>
                      <DialogDescription>
                        Add a new user account with custom storage quota
                      </DialogDescription>
                    </DialogHeader>

                    <form onSubmit={handleCreateUser} className="space-y-4 mt-4">
                      <div className="space-y-2">
                        <Label htmlFor="username">Username</Label>
                        <div className="relative">
                          <Users className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                          <Input
                            id="username"
                            className="pl-10"
                            placeholder="Enter username"
                            value={formData.username}
                            onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                          />
                        </div>
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="email">Email</Label>
                        <div className="relative">
                          <Mail className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                          <Input
                            id="email"
                            type="email"
                            className="pl-10"
                            placeholder="Enter email"
                            value={formData.email}
                            onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                          />
                        </div>
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="password">Password</Label>
                        <div className="relative">
                          <Key className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                          <Input
                            id="password"
                            type="password"
                            className="pl-10"
                            placeholder="Enter password"
                            value={formData.password}
                            onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                          />
                        </div>
                      </div>

                      <div className="space-y-2">
                        <Label htmlFor="quota">Storage Quota (GB)</Label>
                        <div className="relative">
                          <HardDrive className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                          <Input
                            id="quota"
                            type="number"
                            className="pl-10"
                            placeholder="Enter quota in GB"
                            value={formData.max_storage_gb}
                            onChange={(e) => setFormData({ ...formData, max_storage_gb: parseInt(e.target.value) || 10 })}
                          />
                        </div>
                      </div>

                      <div className="flex justify-end space-x-2 pt-4">
                        <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                          Cancel
                        </Button>
                        <Button type="submit">Create User</Button>
                      </div>
                    </form>
                  </DialogContent>
                </Dialog>
              </div>

              {loading ? (
                <div className="text-center py-12">
                  <p className="text-muted-foreground">Loading users...</p>
                </div>
              ) : users.length === 0 ? (
                <div className="text-center py-12 border rounded-lg">
                  <Users className="mx-auto h-12 w-12 text-muted-foreground mb-4" />
                  <h3 className="text-lg font-semibold mb-2">No Users Yet</h3>
                  <p className="text-muted-foreground mb-4">Create your first user to get started</p>
                </div>
              ) : (
                <div className="grid gap-4">
                  {users.map((user) => (
                    <div
                      key={user.id}
                      className="border rounded-lg p-6 hover:shadow-md transition-shadow"
                    >
                      <div className="flex items-start justify-between">
                        <div className="flex-1">
                          <div className="flex items-center gap-3 mb-2">
                            <h3 className="text-lg font-semibold">{user.username}</h3>
                            <span
                              className={`px-2 py-1 rounded-full text-xs font-medium ${
                                user.role === "admin"
                                  ? "bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300"
                                  : "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300"
                              }`}
                            >
                              {user.role}
                            </span>
                            <span
                              className={`px-2 py-1 rounded-full text-xs font-medium ${
                                user.status === "active"
                                  ? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
                                  : "bg-gray-100 text-gray-700 dark:bg-gray-900 dark:text-gray-300"
                              }`}
                            >
                              {user.status}
                            </span>
                          </div>
                          <p className="text-sm text-muted-foreground mb-4">{user.email}</p>

                          <div className="space-y-2">
                            <div className="flex justify-between text-sm">
                              <span className="text-muted-foreground">Storage Used</span>
                              <span className="font-medium">
                                {formatBytes(user.used_storage)} / {formatBytes(user.max_storage)}
                              </span>
                            </div>
                            <div className="w-full bg-gray-200 dark:bg-gray-700 rounded-full h-2">
                              <div
                                className={`h-2 rounded-full transition-all ${
                                  getStoragePercentage(user.used_storage, user.max_storage) > 90
                                    ? "bg-red-500"
                                    : getStoragePercentage(user.used_storage, user.max_storage) > 75
                                    ? "bg-yellow-500"
                                    : "bg-green-500"
                                }`}
                                style={{
                                  width: `${getStoragePercentage(user.used_storage, user.max_storage)}%`,
                                }}
                              />
                            </div>
                            <p className="text-xs text-muted-foreground">
                              {getStoragePercentage(user.used_storage, user.max_storage)}% used
                            </p>
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </main>
          </div>
        </div>
      </SidebarProvider>
    </ThemeProvider>
  );
}
