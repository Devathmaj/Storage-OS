import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/contexts/AuthContext";
import { useToast } from "@/components/ui/use-toast";

export type PeerServer = {
  id: string;
  name: string;
  url: string;
  peer_type: "inbound" | "outbound";
  request_status: "pending" | "accepted" | "rejected" | "revoked";
  quota_allocated: number;
  quota_used: number;
  is_active: boolean;
  last_active?: string;
  requested_at: string;
  accepted_at?: string;
};

export function usePeerServers() {
  const { token } = useAuth();
  const { toast } = useToast();
  const [peers, setPeers] = useState<PeerServer[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchPeers = useCallback(async (type?: "inbound" | "outbound") => {
    if (!token) return;
    setLoading(true);
    try {
      const url = type 
        ? `/api/v1/peers?type=${type}`
        : `/api/v1/peers`;
      
      const response = await fetch(url, {
        headers: { Authorization: `Bearer ${token}` },
      });

      if (!response.ok) throw new Error("Failed to load peer servers");
      
      const data = await response.json();
      setPeers(data.peers ?? []);
    } catch (error) {
      console.error(error);
      toast({
        title: "Unable to fetch peer servers",
        description: (error as Error)?.message,
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  }, [token, toast]);

  const requestConnection = async (name: string, url: string) => {
    if (!token) return;

    try {
      const response = await fetch(`/api/v1/peers/request`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ name, url }),
      });

      if (!response.ok) {
        const error = await response.json().catch(() => ({}));
        throw new Error(error.error || "Failed to request connection");
      }

      toast({
        title: "Connection requested",
        description: `Request sent to ${name}`,
      });

      await fetchPeers();
      return true;
    } catch (error) {
      toast({
        title: "Request failed",
        description: (error as Error)?.message,
        variant: "destructive",
      });
      return false;
    }
  };

  const acceptConnection = async (peerId: string, quotaAllocated: number) => {
    if (!token) return;

    try {
      const response = await fetch(`/api/v1/peers/${peerId}/accept`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ quota_allocated: quotaAllocated }),
      });

      if (!response.ok) throw new Error("Failed to accept connection");

      toast({
        title: "Connection accepted",
        description: "Peer server has been granted access",
      });

      await fetchPeers();
      return true;
    } catch (error) {
      toast({
        title: "Accept failed",
        description: (error as Error)?.message,
        variant: "destructive",
      });
      return false;
    }
  };

  const rejectConnection = async (peerId: string) => {
    if (!token) return;

    try {
      const response = await fetch(`/api/v1/peers/${peerId}/reject`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });

      if (!response.ok) throw new Error("Failed to reject connection");

      toast({
        title: "Connection rejected",
        description: "Peer server request has been declined",
      });

      await fetchPeers();
      return true;
    } catch (error) {
      toast({
        title: "Reject failed",
        description: (error as Error)?.message,
        variant: "destructive",
      });
      return false;
    }
  };

  const deleteConnection = async (peerId: string) => {
    if (!token) return;

    try {
      const response = await fetch(`/api/v1/peers/${peerId}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });

      if (!response.ok) throw new Error("Failed to delete peer");

      toast({
        title: "Peer removed",
        description: "Peer server connection has been deleted",
      });

      await fetchPeers();
      return true;
    } catch (error) {
      toast({
        title: "Delete failed",
        description: (error as Error)?.message,
        variant: "destructive",
      });
      return false;
    }
  };

  return {
    peers,
    loading,
    fetchPeers,
    requestConnection,
    acceptConnection,
    rejectConnection,
    deleteConnection,
  };
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
}
