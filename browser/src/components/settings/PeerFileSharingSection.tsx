import { useState, useEffect, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useAuth } from "@/contexts/AuthContext";
import { usePeerServers, formatBytes } from "@/hooks/usePeerServers";
import { AlertTriangle, Plus, Trash2, Server, ArrowRight, ArrowLeft } from "lucide-react";

interface PeerFileSharingSettings {
  id: number;
  enabled: boolean;
  complete_storage_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export function PeerFileSharingSection() {
  const { token } = useAuth();
  const { peers, loading: peersLoading, fetchPeers, requestConnection, acceptConnection, rejectConnection, deleteConnection } = usePeerServers();
  const [loading, setLoading] = useState(true);
  const [settings, setSettings] = useState<PeerFileSharingSettings | null>(null);
  const [showDisableWarning, setShowDisableWarning] = useState(false);
  const [filesOnPeers, setFilesOnPeers] = useState(0);
  
  // Form state for adding new outbound peer
  const [showOutboundForm, setShowOutboundForm] = useState(false);
  const [newPeerName, setNewPeerName] = useState("");
  const [newPeerURL, setNewPeerURL] = useState("");
  const [submitting, setSubmitting] = useState(false);
  
  // Quota input for accepting inbound peers
  const [quotaInput, setQuotaInput] = useState<Record<string, string>>({});

  // Filter peers by type
  const outboundPeers = peers.filter(p => p.peer_type === "outbound");
  const inboundPeers = peers.filter(p => p.peer_type === "inbound");

  const fetchSettings = useCallback(async () => {
    try {
      const response = await fetch("/api/v1/peer-sharing/settings", {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (response.ok) {
        const data = await response.json();
        setSettings(data.settings);
      }
    } catch (error) {
      console.error("Failed to fetch peer sharing settings:", error);
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    fetchSettings();
    fetchPeers();
  }, [fetchSettings, fetchPeers]);

  const updateSettings = async (enabled: boolean, completeStorageEnabled: boolean) => {
    try {
      const response = await fetch("/api/v1/peer-sharing/settings", {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ enabled, complete_storage_enabled: completeStorageEnabled }),
      });

      if (response.status === 409) {
        const data = await response.json();
        setFilesOnPeers(data.files_on_peers);
        setShowDisableWarning(true);
        return;
      }

      if (response.ok) {
        const data = await response.json();
        setSettings(data);
      }
    } catch (error) {
      console.error("Failed to update settings:", error);
    }
  };

  const confirmDisableCompleteStorage = async () => {
    try {
      const response = await fetch("/api/v1/peer-sharing/settings/confirm-disable", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ confirmed: true }),
      });

      if (response.ok) {
        const data = await response.json();
        setSettings(data);
        setShowDisableWarning(false);
      }
    } catch (error) {
      console.error("Failed to disable complete storage:", error);
    }
  };

  const handleRequestConnection = async () => {
    if (!newPeerName.trim() || !newPeerURL.trim()) {
      alert("Name and URL are required");
      return;
    }

    setSubmitting(true);
    const success = await requestConnection(newPeerName.trim(), newPeerURL.trim());
    setSubmitting(false);

    if (success) {
      setNewPeerName("");
      setNewPeerURL("");
      setShowOutboundForm(false);
    }
  };

  const handleAcceptInbound = async (peerId: string) => {
    const quotaStr = quotaInput[peerId] || "1";
    const quotaGB = parseFloat(quotaStr);
    if (isNaN(quotaGB) || quotaGB <= 0) {
      alert("Please enter a valid quota in GB");
      return;
    }
    const quotaBytes = quotaGB * 1024 * 1024 * 1024;
    await acceptConnection(peerId, quotaBytes);
  };

  if (loading || peersLoading) {
    return (
      <Card>
        <CardContent className="py-6">
          <p className="text-sm text-muted-foreground">Loading peer file sharing settings...</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Server className="h-5 w-5" />
            Peer File Sharing
          </CardTitle>
          <CardDescription>
            Share file storage with peer servers for redundancy and recovery. When enabled, 
            copies of your Reed-Solomon encoded files can be stored on peer servers.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Main Enable Toggle */}
          <div className="flex items-center justify-between rounded-lg border p-4">
            <div className="space-y-0.5">
              <div className="font-medium">Enable Peer File Sharing</div>
              <div className="text-sm text-muted-foreground">
                Allow storing file copies on peer servers and receiving files from peers
              </div>
            </div>
            <Switch
              checked={settings?.enabled || false}
              onCheckedChange={(checked) => updateSettings(checked, settings?.complete_storage_enabled || false)}
            />
          </div>

          {settings?.enabled && (
            <>
              {/* Warning about disabling */}
              <Alert>
                <AlertTriangle className="h-4 w-4" />
                <AlertTitle>Important</AlertTitle>
                <AlertDescription>
                  Disabling peer file sharing will reduce your chances of file recovery in case 
                  of storage node failures. Copies stored on peer servers provide redundancy.
                </AlertDescription>
              </Alert>

              {/* Complete Storage Toggle */}
              <div className="flex items-center justify-between rounded-lg border p-4">
                <div className="space-y-0.5">
                  <div className="font-medium">Enable Peer Server Complete Storage</div>
                  <div className="text-sm text-muted-foreground">
                    Store all Reed-Solomon shards on peer servers, not just copies. 
                    <span className="text-amber-600"> Warning: Disabling will make peer-stored files inaccessible.</span>
                  </div>
                </div>
                <Switch
                  checked={settings?.complete_storage_enabled || false}
                  onCheckedChange={(checked) => updateSettings(settings?.enabled || false, checked)}
                />
              </div>

              {/* Outbound Peers Section */}
              <div className="space-y-3 rounded-lg border p-4">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <ArrowRight className="h-4 w-4 text-blue-500" />
                    <h4 className="font-medium">Servers You Send To (Outbound)</h4>
                  </div>
                  <Button size="sm" onClick={() => setShowOutboundForm(!showOutboundForm)}>
                    {showOutboundForm ? "Cancel" : <><Plus className="h-4 w-4 mr-1" /> Request Connection</>}
                  </Button>
                </div>
                <p className="text-sm text-muted-foreground">
                  These are peer servers where your file copies will be stored for redundancy.
                </p>

                {/* Add new peer form */}
                {showOutboundForm && (
                  <div className="space-y-3 rounded-md border bg-muted/50 p-3">
                    <div className="space-y-2">
                      <label className="text-sm font-medium block">Server Name</label>
                      <Input
                        placeholder="e.g. East Coast Server"
                        value={newPeerName}
                        onChange={(e) => setNewPeerName(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium block">Server URL</label>
                      <Input
                        placeholder="http://192.168.1.100:8081 or http://peer-server.com:8081"
                        value={newPeerURL}
                        onChange={(e) => setNewPeerURL(e.target.value)}
                      />
                    </div>
                    <div className="flex justify-end">
                      <Button onClick={handleRequestConnection} disabled={submitting}>
                        {submitting ? "Requesting..." : "Send Request"}
                      </Button>
                    </div>
                  </div>
                )}

                {outboundPeers.length > 0 ? (
                  <div className="space-y-2">
                    {outboundPeers.map((peer) => (
                      <div
                        key={peer.id}
                        className="flex flex-col gap-2 rounded-md border p-3"
                      >
                        <div className="flex items-start justify-between">
                          <div className="flex-1">
                            <div className="flex items-center gap-2">
                              <span className="font-medium">{peer.name}</span>
                              <Badge variant={peer.is_active ? "secondary" : "outline"}>
                                {peer.is_active ? "Active" : "Inactive"}
                              </Badge>
                              <Badge 
                                variant={
                                  peer.request_status === "accepted" ? "default" :
                                  peer.request_status === "pending" ? "secondary" :
                                  "destructive"
                                }
                              >
                                {peer.request_status}
                              </Badge>
                            </div>
                            <p className="text-sm text-muted-foreground">{peer.url}</p>
                            <div className="mt-1 text-xs text-muted-foreground">
                              <span>Requested: {new Date(peer.requested_at).toLocaleString()}</span>
                              {peer.request_status === "accepted" && peer.accepted_at && (
                                <>
                                  <span className="mx-2">•</span>
                                  <span>Accepted: {new Date(peer.accepted_at).toLocaleString()}</span>
                                </>
                              )}
                              {peer.request_status === "accepted" && (
                                <>
                                  <span className="mx-2">•</span>
                                  <span>Available: {formatBytes(peer.quota_allocated - peer.quota_used)} / {formatBytes(peer.quota_allocated)}</span>
                                </>
                              )}
                            </div>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => deleteConnection(peer.id)}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                        {peer.request_status === "pending" && (
                          <p className="text-xs text-amber-600">
                            ⏳ Waiting for {peer.name} to accept your connection request
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground py-2">
                    No outbound peers configured. Click "Request Connection" to add peers for file redundancy.
                  </p>
                )}
              </div>

              {/* Inbound Peers Section */}
              <div className="space-y-3 rounded-lg border p-4">
                <div className="flex items-center gap-2">
                  <ArrowLeft className="h-4 w-4 text-green-500" />
                  <h4 className="font-medium">Servers Sending to You (Inbound)</h4>
                </div>
                <p className="text-sm text-muted-foreground">
                  These are peer servers that have requested to store their file copies on your storage nodes.
                  Accept and allocate quota for them.
                </p>

                {inboundPeers.length > 0 ? (
                  <div className="space-y-2">
                    {inboundPeers.map((peer) => (
                      <div
                        key={peer.id}
                        className="flex flex-col gap-2 rounded-md border p-3"
                      >
                        <div className="flex items-start justify-between">
                          <div className="flex-1">
                            <div className="flex items-center gap-2">
                              <span className="font-medium">{peer.name}</span>
                              <Badge variant={peer.is_active ? "secondary" : "outline"}>
                                {peer.is_active ? "Active" : "Inactive"}
                              </Badge>
                              <Badge 
                                variant={
                                  peer.request_status === "accepted" ? "default" :
                                  peer.request_status === "pending" ? "secondary" :
                                  "destructive"
                                }
                              >
                                {peer.request_status}
                              </Badge>
                            </div>
                            <p className="text-sm text-muted-foreground">{peer.url}</p>
                            <div className="mt-1 text-xs text-muted-foreground">
                              <span>Requested: {new Date(peer.requested_at).toLocaleString()}</span>
                              {peer.request_status === "accepted" && (
                                <>
                                  <span className="mx-2">•</span>
                                  <span>Quota: {formatBytes(peer.quota_used)} / {formatBytes(peer.quota_allocated)}</span>
                                </>
                              )}
                            </div>
                          </div>
                        </div>

                        {peer.request_status === "pending" && (
                          <div className="flex items-end gap-2 pt-2 border-t">
                            <div className="flex-1">
                              <label className="text-xs font-medium block mb-1">Allocate Quota (GB)</label>
                              <Input
                                type="number"
                                min={0.1}
                                step={0.1}
                                placeholder="1.0"
                                value={quotaInput[peer.id] || ""}
                                onChange={(e) => setQuotaInput({ ...quotaInput, [peer.id]: e.target.value })}
                              />
                            </div>
                            <Button size="sm" onClick={() => handleAcceptInbound(peer.id)}>
                              Accept
                            </Button>
                            <Button size="sm" variant="outline" onClick={() => rejectConnection(peer.id)}>
                              Reject
                            </Button>
                          </div>
                        )}

                        {peer.request_status === "accepted" && (
                          <div className="flex justify-end pt-2 border-t">
                            <Button size="sm" variant="destructive" onClick={() => deleteConnection(peer.id)}>
                              Revoke Access
                            </Button>
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-sm text-muted-foreground py-2">
                    No inbound peer requests. Other servers will appear here when they request to send files to you.
                  </p>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* Disable Complete Storage Warning Dialog */}
      <Dialog open={showDisableWarning} onOpenChange={setShowDisableWarning}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-amber-600">
              <AlertTriangle className="h-5 w-5" />
              Warning: Data Loss Risk
            </DialogTitle>
            <DialogDescription>
              You have <strong>{filesOnPeers}</strong> file shards stored on peer servers. 
              Disabling complete storage will make these files inaccessible.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <p className="text-sm text-muted-foreground">
              Before disabling, you should:
            </p>
            <ul className="list-disc list-inside text-sm text-muted-foreground mt-2 space-y-1">
              <li>Download all data stored on peer servers</li>
              <li>Ensure you have local copies of important files</li>
              <li>Consider keeping complete storage enabled for redundancy</li>
            </ul>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDisableWarning(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={confirmDisableCompleteStorage}>
              I understand, disable anyway
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
