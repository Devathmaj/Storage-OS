import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { usePeerServers, formatBytes, type PeerServer } from "@/hooks/usePeerServers";

export function InboundPeersSection() {
  const { peers, loading, fetchPeers, acceptConnection, rejectConnection, deleteConnection } = usePeerServers();
  const [quotaInput, setQuotaInput] = useState<Record<string, string>>({});

  const inboundPeers = peers.filter(p => p.peer_type === "inbound");

  useEffect(() => {
    fetchPeers("inbound");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleAccept = async (peerId: string) => {
    const quotaStr = quotaInput[peerId] || "1";
    const quotaGB = parseFloat(quotaStr);
    if (isNaN(quotaGB) || quotaGB <= 0) {
      alert("Please enter a valid quota in GB");
      return;
    }
    const quotaBytes = quotaGB * 1024 * 1024 * 1024;
    await acceptConnection(peerId, quotaBytes);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Servers Sending to You (Inbound)</CardTitle>
        <CardDescription>
          Peer servers that have requested to send files to your storage. Accept and allocate quota for them.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : inboundPeers.length === 0 ? (
          <p className="text-sm text-muted-foreground">No inbound peer requests.</p>
        ) : (
          <div className="space-y-3">
            {inboundPeers.map((peer) => (
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
                      {peer.request_status === "accepted" && (
                        <>
                          <span className="mx-2">•</span>
                          <span>Quota: {formatBytes(peer.quota_used)} / {formatBytes(peer.quota_allocated)}</span>
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
                </div>

                {peer.request_status === "pending" && (
                  <div className="flex items-end gap-2">
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
                    <Button size="sm" onClick={() => handleAccept(peer.id)}>
                      Accept
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => rejectConnection(peer.id)}>
                      Reject
                    </Button>
                  </div>
                )}

                {peer.request_status === "accepted" && (
                  <div className="flex justify-end">
                    <Button size="sm" variant="destructive" onClick={() => deleteConnection(peer.id)}>
                      Revoke Access
                    </Button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
