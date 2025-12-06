import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { ShieldCheck, ShieldX, RefreshCw, Calendar, AlertTriangle } from 'lucide-react';
import { toast } from 'sonner';
import { useAuth } from '@/contexts/AuthContext';

interface OSNode {
  id: string;
  certificate_cn: string;
  certificate_fingerprint: string;
  enrolled_at: string;
  certificate_expires_at: string;
  last_seen: string | null;
  status: string;
  parsed_system_info: {
    hostname: string;
    cpu: string;
    os_version: string;
    network_id: string;
  };
  revoked_at: string | null;
  revocation_reason: string | null;
}

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8081';

export const EnrolledNodesSection: React.FC = () => {
  const { token } = useAuth();
  const [nodes, setNodes] = useState<OSNode[]>([]);
  const [loading, setLoading] = useState(false);

  // Load enrolled nodes
  const loadNodes = async () => {
    setLoading(true);
    try {
      const response = await fetch(`${API_URL}/v1/provisioning/nodes`, {
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to load nodes');
      }

      const data = await response.json();
      setNodes(data.nodes || []);
    } catch (error) {
      console.error('Error loading nodes:', error);
      toast.error('Failed to load enrolled nodes');
    } finally {
      setLoading(false);
    }
  };

  // Revoke node
  const handleRevoke = async (nodeId: string) => {
    const reason = prompt('Enter revocation reason:');
    if (!reason) return;

    try {
      const response = await fetch(`${API_URL}/v1/provisioning/nodes/${nodeId}/revoke`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ reason }),
      });

      if (!response.ok) {
        throw new Error('Failed to revoke node');
      }

      toast.success('Node certificate revoked');
      loadNodes();
    } catch (error) {
      console.error('Error revoking node:', error);
      toast.error('Failed to revoke node');
    }
  };

  // Get status badge
  const getStatusBadge = (node: OSNode) => {
    switch (node.status) {
      case 'active':
        return <Badge className="bg-green-500">Active</Badge>;
      case 'revoked':
        return <Badge variant="destructive">Revoked</Badge>;
      case 'expired':
        return <Badge variant="secondary">Expired</Badge>;
      default:
        return <Badge variant="outline">{node.status}</Badge>;
    }
  };

  // Check if certificate is expiring soon
  const isExpiringSoon = (expiresAt: string) => {
    const daysUntilExpiry = (new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24);
    return daysUntilExpiry < 30 && daysUntilExpiry > 0;
  };

  // Check if node is online (last seen within 5 minutes)
  const isOnline = (lastSeen: string | null) => {
    if (!lastSeen) return false;
    const minutes = (Date.now() - new Date(lastSeen).getTime()) / (1000 * 60);
    return minutes < 5;
  };

  // Auto-refresh nodes
  useEffect(() => {
    loadNodes();
    const interval = setInterval(loadNodes, 30000); // Refresh every 30 seconds
    return () => clearInterval(interval);
  }, []);

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheck className="w-5 h-5" />
              Enrolled OS Nodes
            </CardTitle>
            <CardDescription>
              Manage enrolled OS nodes and their certificates
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="icon"
            onClick={loadNodes}
            disabled={loading}
          >
            <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {nodes.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            No enrolled nodes yet
          </div>
        ) : (
          <div className="space-y-4">
            {/* Statistics */}
            <div className="grid grid-cols-4 gap-4">
              <Card>
                <CardContent className="p-4">
                  <div className="text-2xl font-bold">{nodes.filter(n => n.status === 'active').length}</div>
                  <div className="text-sm text-muted-foreground">Active Nodes</div>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="p-4">
                  <div className="text-2xl font-bold">{nodes.filter(n => isOnline(n.last_seen)).length}</div>
                  <div className="text-sm text-muted-foreground">Online Now</div>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="p-4">
                  <div className="text-2xl font-bold">{nodes.filter(n => isExpiringSoon(n.certificate_expires_at)).length}</div>
                  <div className="text-sm text-muted-foreground">Expiring Soon</div>
                </CardContent>
              </Card>
              <Card>
                <CardContent className="p-4">
                  <div className="text-2xl font-bold">{nodes.filter(n => n.status === 'revoked').length}</div>
                  <div className="text-sm text-muted-foreground">Revoked</div>
                </CardContent>
              </Card>
            </div>

            {/* Nodes Table */}
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Node ID</TableHead>
                  <TableHead>Hostname</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Certificate</TableHead>
                  <TableHead>Last Seen</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.map((node) => (
                  <TableRow key={node.id}>
                    <TableCell className="font-mono text-sm">
                      {node.id}
                    </TableCell>
                    <TableCell>
                      <div>
                        <div className="font-medium">{node.parsed_system_info?.hostname || node.certificate_cn}</div>
                        <div className="text-sm text-muted-foreground">
                          {node.parsed_system_info?.os_version || 'Unknown OS'}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="space-y-1">
                        {getStatusBadge(node)}
                        {isOnline(node.last_seen) && node.status === 'active' && (
                          <Badge variant="outline" className="ml-1">
                            <div className="w-2 h-2 bg-green-500 rounded-full mr-1 animate-pulse" />
                            Online
                          </Badge>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="text-sm space-y-1">
                        <div className="flex items-center gap-1">
                          <Calendar className="w-3 h-3" />
                          Expires: {new Date(node.certificate_expires_at).toLocaleDateString()}
                        </div>
                        {isExpiringSoon(node.certificate_expires_at) && node.status === 'active' && (
                          <div className="flex items-center gap-1 text-amber-500">
                            <AlertTriangle className="w-3 h-3" />
                            Expiring soon
                          </div>
                        )}
                        <div className="font-mono text-xs text-muted-foreground">
                          {node.certificate_fingerprint.substring(0, 16)}...
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {node.last_seen ? (
                        <div className="text-sm">
                          {new Date(node.last_seen).toLocaleString()}
                        </div>
                      ) : (
                        <span className="text-muted-foreground">Never</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      {node.status === 'active' ? (
                        <Button
                          size="sm"
                          variant="destructive"
                          onClick={() => handleRevoke(node.id)}
                          className="gap-1"
                        >
                          <ShieldX className="w-4 h-4" />
                          Revoke
                        </Button>
                      ) : node.status === 'revoked' ? (
                        <div className="text-sm text-muted-foreground">
                          Revoked: {node.revocation_reason}
                        </div>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
};
