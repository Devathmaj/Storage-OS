import { useState, useEffect, useMemo, useCallback } from "react";
import { ThemeProvider } from "next-themes";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Navbar } from "@/components/Navbar";
import { FileGrid, FileItem } from "@/components/FileGrid";
import { useAuth } from "@/contexts/AuthContext";
import { useToast } from "@/hooks/use-toast";

const API_BASE = "http://localhost:8081/v1/browser";

interface PreviewData {
  url?: string;
  text?: string;
  mimeType: string;
}

const Favorites = () => {
  const [files, setFiles] = useState<FileItem[]>([]);
  const [breadcrumbs] = useState(["Favorites"]);
  const [previewCache, setPreviewCache] = useState<Record<string, PreviewData>>({});
  const { token, isAuthenticated } = useAuth();
  const { toast } = useToast();

  const previewUrls = useMemo(() => {
    const map: Record<string, string> = {};
    Object.entries(previewCache).forEach(([id, data]) => {
      if (data?.url) {
        map[id] = data.url;
      }
    });
    return map;
  }, [previewCache]);

  const fetchFavorites = async () => {
    try {
      const response = await fetch(`${API_BASE}/favorites`, {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error("Failed to fetch favorites");
      }

      const data = await response.json();
      
      // Transform backend ContentItem[] to FileItem[]
      const items: FileItem[] = data.items.map((item: any) => ({
        id: item.id,
        name: item.name,
        type: item.type,
        fileType: item.file_type,
        size: item.size ? formatSize(item.size) : undefined,
        modified: item.modified,
        isFavorite: true,
        nodeId: item.node_id,
        isCompressed: item.is_compressed,
        previewable: item.previewable,
        mimeType: item.mime_type,
      }));

      setFiles(items);
    } catch (error) {
      console.error("Error fetching favorites:", error);
      toast({
        title: "Error",
        description: "Failed to load favorites",
        variant: "destructive",
      });
    }
  };

  useEffect(() => {
    if (token) {
      fetchFavorites();
    }
  }, [token]);

  useEffect(() => {
    files.forEach((item) => {
      if (
        item.type === "file" &&
        !item.isCompressed &&
        item.previewable &&
        item.fileType === "image" &&
        !previewCache[item.id]
      ) {
        fetchPreview(item, { silent: true });
      }
    });
  }, [files]);

  const fetchPreview = useCallback(async (item: FileItem, options?: { silent?: boolean }) => {
    if (item.type !== "file" || item.isCompressed || !item.previewable) {
      return null;
    }

    if (!isAuthenticated || !token) {
      return null;
    }

    if (previewCache[item.id]) {
      return previewCache[item.id];
    }

    try {
      const response = await fetch(`${API_BASE}/preview?file_id=${item.id}`, {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error(`Preview failed with status ${response.status}`);
      }

      const contentType = response.headers.get("Content-Type") || "";
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const data: PreviewData = { url, mimeType: contentType };

      setPreviewCache((prev) => {
        const next = { ...prev };
        const existing = next[item.id];
        if (existing?.url) {
          URL.revokeObjectURL(existing.url);
        }
        next[item.id] = data;
        return next;
      });

      return data;
    } catch (error) {
      if (!options?.silent) {
        console.error("Failed to load preview", error);
      }
      return null;
    }
  }, [isAuthenticated, previewCache, token]);

  const formatSize = (bytes: number): string => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  const handleRemoveFavorite = async (id: string, type: string) => {
    try {
      const response = await fetch(`${API_BASE}/favorites`, {
        method: "DELETE",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          id: id,
          type: type,
        }),
      });

      if (!response.ok) {
        throw new Error("Failed to remove favorite");
      }

      toast({
        title: "Removed from favorites",
        description: "Item removed successfully",
      });

      // Refresh the list
      fetchFavorites();
    } catch (error) {
      console.error("Error removing favorite:", error);
      toast({
        title: "Error",
        description: "Failed to remove favorite",
        variant: "destructive",
      });
    }
  };

  const downloadItem = async (item: FileItem) => {
    if (!isAuthenticated || !token) {
      toast({
        title: "Error",
        description: "Please login to download items",
        variant: "destructive",
      });
      return;
    }

    const isFolder = item.type === "folder";
    const endpoint = isFolder
      ? `${API_BASE}/download-folder?folder_id=${item.id}`
      : `${API_BASE}/download?file_id=${item.id}`;

    try {
      const response = await fetch(endpoint, {
        headers: {
          Authorization: `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error(`Download failed with status ${response.status}`);
      }

      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      const baseName = item.name?.trim() || (isFolder ? "folder" : "file");
      const downloadName = isFolder
        ? (baseName.toLowerCase().endsWith(".zip") ? baseName : `${baseName}.zip`)
        : baseName;

      link.href = url;
      link.download = downloadName;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);

      toast({
        title: "Success",
        description: `Started download for ${item.name}`,
      });
    } catch (error) {
      console.error("Download failed", error);
      toast({
        title: "Error",
        description: `Failed to download ${item.name}`,
        variant: "destructive",
      });
    }
  };

  const deleteItem = async (item: FileItem) => {
    if (!isAuthenticated || !token) {
      toast({
        title: "Error",
        description: "Please login to delete items",
        variant: "destructive",
      });
      return;
    }

    try {
      const response = await fetch(`${API_BASE}/trash`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ id: item.id, type: item.type }),
      });

      if (!response.ok) {
        throw new Error('Failed to move item to trash');
      }

      toast({
        title: "Moved to trash",
        description: `${item.name} moved to trash`,
      });
      
      fetchFavorites(); // Refresh the list
    } catch (error) {
      console.error('Error moving item to trash:', error);
      toast({
        title: "Error",
        description: `Failed to delete ${item.name}`,
        variant: "destructive",
      });
    }
  };

  const handleItemAction = (action: string, item: FileItem) => {
    switch (action) {
      case "preview":
        if (item.isCompressed) {
          toast({
            title: "Info",
            description: "Compressed files must be downloaded to view.",
          });
        } else {
          toast({
            title: "Info",
            description: "Preview not available on favorites page. Please open from home.",
          });
        }
        break;
      case "rename":
        toast({
          title: "Info",
          description: "Rename functionality would go here",
        });
        break;
      case "download":
        downloadItem(item);
        break;
      case "delete":
        deleteItem(item);
        break;
      case "unfavorite":
        handleRemoveFavorite(item.id, item.type);
        break;
    }
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

            <Navbar breadcrumbs={breadcrumbs} onSearch={() => {}} />

            <main className="p-6">
              <h1 className="text-2xl font-semibold mb-4">Favorites</h1>
              <FileGrid items={files} onItemClick={() => {}} onItemAction={handleItemAction} previewUrls={previewUrls} />
            </main>
          </div>
        </div>
      </SidebarProvider>
    </ThemeProvider>
  );
};

export default Favorites;
