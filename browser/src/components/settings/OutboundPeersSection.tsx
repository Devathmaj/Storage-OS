import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { usePeerServers, formatBytes, type PeerServer } from "@/hooks/usePeerServers";

export function OutboundPeersSection() {
  const { peers, loading, fetchPeers, requestConnection, deleteConnection } = usePeerServers();
  const [showForm, setShowForm] = useState(false);
  const [peerName, setPeerName] = useState("");
  const [peerURL, setPeerURL] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const outboundPeers = peers.filter(p => p.peer_type === "outbound");

  useEffect(() => {
    fetchPeers("outbound");
  }, []);

  const handleSubmit = async () => {
    if (!peerName.trim() || !peerURL.trim()) {
      alert("Name and URL are required");
      return;
    }

    setSubmitting(true);
    const success = await requestConnection(peerName.trim(), peerURL.trim());
    setSubmitting(false);

    if (success) {
      setPeerName("");
      setPeerURL("");
      setShowForm(false);
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between space-y-0">
        <div>
          <CardTitle>Servers You Send To (Outbound)</CardTitle>
          <CardDescription>
            Request connections to other servers to store your files on their storage.
          </CardDescription>
        </div>
        <Button onClick={() => setShowForm(!showForm)}>
          {showForm ? "Hide" : "Request Connection"}
        </Button>
      </CardHeader>
      <CardContent>
        {showForm && (
          <div className="mb-4 space-y-3 rounded-md border p-3">
            <div className="space-y-2">
              <label className="text-sm font-medium block">Server Name</label>
              <Input
                placeholder="e.g. East Coast Server"
                value={peerName}
                onChange={(e) => setPeerName(e.target.value)}
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium block">Server URL</label>
              <Input
                placeholder="http://peer-server.com:8081"
                value={peerURL}
                onChange={(e) => setPeerURL(e.target.value)}
              />
            </div>
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setShowForm(false)}>
                Cancel
              </Button>
              <Button onClick={handleSubmit} disabled={submitting}>
                {submitting ? "Requesting..." : "Send Request"}
              </Button>
            </div>
          </div>
        )}

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : outboundPeers.length === 0 ? (
          <p className="text-sm text-muted-foreground">No outbound connections requested yet.</p>
        ) : (
          <div className="space-y-3">
            {outboundPeers.map((peer) => (
              <div key={peer.id} className="flex flex-col gap-3 rounded-md border p-3">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <p className="font-medium">{peer.name}</p>
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
                      {peer.last_active && (
                        <>
                          <span className="mx-2">•</span>
                          <span>Last Active: {new Date(peer.last_active).toLocaleString()}</span>
                        </>
                      )}
                    </div>
                  </div>
                  <Button size="sm" variant="ghost" onClick={() => deleteConnection(peer.id)}>
                    Remove
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
        )}
      </CardContent>
    </Card>
  );
}
