import { useEffect, useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { RefreshCw, HardDrive, Server, Cloud } from "lucide-react";
import { useAuth } from "@/contexts/AuthContext";
import { useToast } from "@/hooks/use-toast";

interface StorageNodeStats {
  id: string;
  name: string;
  url?: string;
  quota_mb: number;
  used_mb: number;
  available_mb: number;
  used_percent: number;
  is_active: boolean;
  is_online?: boolean;
  last_active?: string;
  status?: string;
}

interface PeerServerStats {
  id: string;
  name: string;
  url: string;
  quota_allocated: number;
  quota_used: number;
  storage_remaining: number;
  used_percent: number;
  is_active: boolean;
  last_active?: string;
}

interface StorageOverview {
  total_storage_mb: number;
  total_used_mb: number;
  total_available_mb: number;
  total_used_percent: number;
  total_file_size_mb?: number;
  total_file_count?: number;
  storage_nodes: StorageNodeStats[];
  active_node_count: number;
  total_node_count: number;
  peer_servers?: PeerServerStats[];
  peer_storage_used_mb?: number;
  peer_quota_total_mb?: number;
}

interface UserStorage {
  used_storage_bytes: number;
  max_storage_bytes: number;
  used_percent: number;
  file_count: number;
  folder_count: number;
  trash_size_bytes: number;
  trash_count: number;
}

export function StorageSection() {
  const { token } = useAuth();
  const { toast } = useToast();
  const [storageOverview, setStorageOverview] = useState<StorageOverview | null>(null);
  const [userStorage, setUserStorage] = useState<UserStorage | null>(null);
  const [loading, setLoading] = useState(true);

  const fetchStorageData = async () => {
    try {
      setLoading(true);
      
      // Fetch storage overview
      const overviewRes = await fetch("/api/v1/settings/storage/overview", {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (overviewRes.ok) {
        const data = await overviewRes.json();
        setStorageOverview(data);
      }
      
      // Fetch user storage
      const userRes = await fetch("/api/v1/settings/storage/user", {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (userRes.ok) {
        const data = await userRes.json();
        setUserStorage(data);
      }
    } catch (error) {
      console.error("Failed to fetch storage data:", error);
      toast({
        title: "Error",
        description: "Failed to fetch storage statistics",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (token) {
      fetchStorageData();
    }
  }, [token]);

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  const formatMB = (mb: number) => {
    if (mb < 1024) return `${mb.toFixed(1)} MB`;
    if (mb < 1024 * 1024) return `${(mb / 1024).toFixed(2)} GB`;
    return `${(mb / (1024 * 1024)).toFixed(2)} TB`;
  };

  const getUsageColor = (percent: number) => {
    if (percent >= 90) return "text-red-500";
    if (percent >= 70) return "text-yellow-500";
    return "text-green-500";
  };

  const getProgressColor = (percent: number) => {
    if (percent >= 90) return "bg-red-500";
    if (percent >= 70) return "bg-yellow-500";
    return "bg-green-500";
  };

  return (
    <div className="space-y-6">
      {/* Header with Refresh */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Storage Overview</h2>
          <p className="text-sm text-muted-foreground">
            Monitor your storage usage across all nodes
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={fetchStorageData} disabled={loading}>
          <RefreshCw className={`h-4 w-4 mr-2 ${loading ? "animate-spin" : ""}`} />
          Refresh
        </Button>
      </div>

      {/* User Storage Card */}
      {userStorage && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HardDrive className="h-5 w-5" />
              Your Storage
            </CardTitle>
            <CardDescription>
              Personal storage usage and limits
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {userStorage.max_storage_bytes > 0 ? (
              <>
                <div className="flex justify-between items-center">
                  <span className="text-sm text-muted-foreground">Used</span>
                  <span className={`text-lg font-semibold ${getUsageColor(userStorage.used_percent)}`}>
                    {formatBytes(userStorage.used_storage_bytes)} / {formatBytes(userStorage.max_storage_bytes)}
                  </span>
                </div>
                <Progress 
                  value={userStorage.used_percent} 
                  className={`h-3 ${getProgressColor(userStorage.used_percent)}`}
                />
                <div className="flex justify-between text-sm text-muted-foreground">
                  <span>{userStorage.used_percent.toFixed(1)}% used</span>
                  <span>{formatBytes(userStorage.max_storage_bytes - userStorage.used_storage_bytes)} available</span>
                </div>
              </>
            ) : (
              <>
                <div className="flex justify-between items-center">
                  <span className="text-sm text-muted-foreground">Used</span>
                  <span className="text-lg font-semibold">
                    {formatBytes(userStorage.used_storage_bytes)}
                  </span>
                </div>
                <div className="text-sm text-muted-foreground">
                  <Badge variant="secondary">Unlimited Storage</Badge>
                </div>
              </>
            )}
            
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4 pt-4 border-t">
              <div className="text-center">
                <div className="text-2xl font-bold">{userStorage.file_count}</div>
                <div className="text-sm text-muted-foreground">Files</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold">{userStorage.folder_count}</div>
                <div className="text-sm text-muted-foreground">Folders</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold">{userStorage.trash_count}</div>
                <div className="text-sm text-muted-foreground">In Trash</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold">{formatBytes(userStorage.trash_size_bytes)}</div>
                <div className="text-sm text-muted-foreground">Trash Size</div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* System Storage Overview */}
      {storageOverview && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server className="h-5 w-5" />
              System Storage
            </CardTitle>
            <CardDescription>
              Total storage across all {storageOverview.total_node_count} storage nodes 
              ({storageOverview.active_node_count} active)
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex justify-between items-center">
              <span className="text-sm text-muted-foreground">Total Used</span>
              <span className={`text-lg font-semibold ${getUsageColor(storageOverview.total_used_percent)}`}>
                {formatMB(storageOverview.total_used_mb)} / {formatMB(storageOverview.total_storage_mb)}
              </span>
            </div>
            <Progress 
              value={storageOverview.total_used_percent} 
              className={`h-3 ${getProgressColor(storageOverview.total_used_percent)}`}
            />
            <div className="flex justify-between text-sm text-muted-foreground">
              <span>{storageOverview.total_used_percent.toFixed(1)}% used</span>
              <span>{formatMB(storageOverview.total_available_mb)} available</span>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Per-Node Storage */}
      {storageOverview && storageOverview.storage_nodes.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Storage Nodes</CardTitle>
            <CardDescription>
              Data stored on each node (from file shards)
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {storageOverview.storage_nodes.map((node) => {
                // Cap percentage at 100 for display
                const displayPercent = Math.min(node.used_percent, 100);
                const isOverQuota = node.used_percent > 100;
                
                return (
                  <div key={node.id} className="p-4 border rounded-lg">
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <Server className="h-4 w-4" />
                        <span className="font-medium">{node.name || `Node ${node.id}`}</span>
                        <Badge variant={node.is_active || node.status === "active" ? "default" : "secondary"}>
                          {node.is_online ? "Online" : node.is_active || node.status === "active" ? "Active" : "Offline"}
                        </Badge>
                        {isOverQuota && (
                          <Badge variant="destructive">Over Quota</Badge>
                        )}
                      </div>
                      <span className={`text-sm ${isOverQuota ? "text-red-500" : getUsageColor(node.used_percent)}`}>
                        {node.used_percent.toFixed(1)}%
                      </span>
                    </div>
                    <Progress value={displayPercent} className={`h-2 mb-2 ${isOverQuota ? "bg-red-200" : ""}`} />
                    <div className="flex justify-between text-xs text-muted-foreground">
                      <span>{formatMB(node.used_mb)} stored</span>
                      <span>{formatMB(node.quota_mb)} quota</span>
                    </div>
                    {node.url && (
                      <div className="text-xs text-muted-foreground mt-1">{node.url}</div>
                    )}
                    <div className="text-xs text-muted-foreground mt-1">ID: {node.id}</div>
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Peer Server Storage */}
      {storageOverview && storageOverview.peer_servers && storageOverview.peer_servers.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Cloud className="h-5 w-5" />
              Peer Server Storage
            </CardTitle>
            <CardDescription>
              Storage allocated on peer servers for file sharing
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {storageOverview.peer_servers.map((peer) => (
                <div key={peer.id} className="p-4 border rounded-lg">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <Cloud className="h-4 w-4" />
                      <span className="font-medium">{peer.name}</span>
                      <Badge variant={peer.is_active ? "default" : "secondary"}>
                        {peer.is_active ? "Online" : "Offline"}
                      </Badge>
                    </div>
                    <span className={`text-sm ${getUsageColor(peer.used_percent)}`}>
                      {peer.used_percent.toFixed(1)}%
                    </span>
                  </div>
                  <Progress value={peer.used_percent} className="h-2 mb-2" />
                  <div className="flex justify-between text-xs text-muted-foreground">
                    <span>{formatBytes(peer.quota_used)} used</span>
                    <span>{formatBytes(peer.quota_allocated)} allocated</span>
                  </div>
                  <div className="text-xs text-muted-foreground mt-1">{peer.url}</div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Empty State */}
      {!loading && (!storageOverview || storageOverview.storage_nodes.length === 0) && (
        <Card>
          <CardContent className="py-10 text-center">
            <Server className="h-12 w-12 mx-auto text-muted-foreground mb-4" />
            <h3 className="text-lg font-medium mb-2">No Storage Nodes</h3>
            <p className="text-sm text-muted-foreground">
              Add storage nodes in the OS Storage Nodes tab to start tracking storage.
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
