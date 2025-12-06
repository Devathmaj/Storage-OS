import { useState, useEffect, useMemo, useCallback, useRef } from "react";
import { ThemeProvider } from "next-themes";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Navbar } from "@/components/Navbar";
import { FileGrid, FileItem } from "@/components/FileGrid";
import { FileList } from "@/components/FileList";
import { UploadControls } from "@/components/UploadControls";
import { UploadQueue, UploadItem } from "@/components/UploadQueue";
import { FilePreviewModal, PreviewData } from "@/components/FilePreviewModal";
import { DropZone } from "@/components/DropZone";
import { toast } from "sonner";
import { useAuth } from "@/contexts/AuthContext";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { LayoutGrid, List as ListIcon } from "lucide-react";

const mockFiles: FileItem[] = [
  {
    id: "1",
    name: "Documents",
    type: "folder",
    modified: "2 days ago",
  },
  {
    id: "2",
    name: "Projects",
    type: "folder",
    modified: "1 week ago",
  },
  {
    id: "3",
    name: "Photos",
    type: "folder",
    modified: "3 days ago",
  },
  {
    id: "4",
    name: "vacation.jpg",
    type: "file",
    fileType: "image",
    size: "2.4 MB",
    modified: "Yesterday",
  },
  {
    id: "5",
    name: "presentation.pdf",
    type: "file",
    fileType: "text",
    size: "1.8 MB",
    modified: "5 hours ago",
  },
  {
    id: "6",
    name: "meeting-recording.mp4",
    type: "file",
    fileType: "video",
    size: "45.2 MB",
    modified: "2 days ago",
  },
];

const API_BASE = "http://localhost:8081/v1/browser";

const Index = () => {
  const { token, isAuthenticated } = useAuth();
  const [files, setFiles] = useState<FileItem[]>([]);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [previewFile, setPreviewFile] = useState<FileItem | null>(null);
  const [currentFolderId, setCurrentFolderId] = useState<string | null>(null);
  const [compressFiles, setCompressFiles] = useState(true);
  const [compressFolders, setCompressFolders] = useState(true);
  const [loading, setLoading] = useState(false);
  const [previewCache, setPreviewCache] = useState<Record<string, PreviewData>>({});
  const [previewLoading, setPreviewLoading] = useState(false);
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  const [breadcrumbStack, setBreadcrumbStack] = useState<{ id: string | null; label: string }[]>([
    { id: null, label: "My Drive" },
  ]);
  const previewCacheRef = useRef<Record<string, PreviewData>>({});
  const previewUrls = useMemo(() => {
    const map: Record<string, string> = {};
    Object.entries(previewCache).forEach(([id, data]) => {
      if (data?.url) {
        map[id] = data.url;
      }
    });
    return map;
  }, [previewCache]);
  const breadcrumbLabels = useMemo(() => breadcrumbStack.map((crumb) => crumb.label), [breadcrumbStack]);

  useEffect(() => {
    previewCacheRef.current = previewCache;
  }, [previewCache]);

  // Fetch files and folders from controller
  const fetchContents = async (folderId: string | null = null) => {
    if (!isAuthenticated || !token) {
      return;
    }

    setLoading(true);
    try {
      const url = folderId 
        ? `${API_BASE}/folder/contents?folder_id=${folderId}`
        : `${API_BASE}/folder/contents`;
      
      const response = await fetch(url, {
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      });

      if (!response.ok) {
        throw new Error('Failed to fetch contents');
      }

      const data = await response.json();
      
      // Map backend data to frontend FileItem format
      const mappedFiles: FileItem[] = (data.contents || []).map((item: any) => ({
        id: item.id,
        name: item.name,
        type: item.type,
        fileType: item.file_type,
        size: item.size ? formatFileSize(item.size) : undefined,
        modified: item.modified,
        extension: item.extension,
        nodeId: item.node_id,
        isCompressed: item.is_compressed,
        previewable: item.previewable,
        mimeType: item.mime_type,
        isFavorite: item.is_favorite || false,
      }));

      setFiles(mappedFiles);
    } catch (error) {
      console.error('Error fetching contents:', error);
      toast.error('Failed to load files');
    } finally {
      setLoading(false);
    }
  };

  // Load contents on mount and when auth changes
  useEffect(() => {
    if (isAuthenticated && token) {
      fetchContents(currentFolderId);
    }
  }, [isAuthenticated, token, currentFolderId]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const stored = window.localStorage.getItem("cloudlet:viewMode");
    if (stored === "grid" || stored === "list") {
      setViewMode(stored);
    }
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    window.localStorage.setItem("cloudlet:viewMode", viewMode);
  }, [viewMode]);

  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  };

  const fetchPreview = useCallback(async (item: FileItem, options?: { silent?: boolean }) => {
    if (item.type !== "file" || item.isCompressed || !item.previewable) {
      if (!options?.silent) {
        toast.info("Preview not available for this item");
      }
      return null;
    }

    if (!isAuthenticated || !token) {
      if (!options?.silent) {
        toast.error("Please login to preview files");
      }
      return null;
    }

    if (previewCache[item.id]) {
      return previewCache[item.id];
    }

    if (!options?.silent) {
      setPreviewLoading(true);
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
      const isText = contentType.startsWith("text/") || contentType.includes("json") || contentType.includes("xml");
      let data: PreviewData;

      if (isText) {
        const text = await response.text();
        data = { text, mimeType: contentType };
      } else {
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        data = { url, mimeType: contentType };
      }

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
      console.error("Failed to load preview", error);
      if (!options?.silent) {
        toast.error(`Unable to load preview for ${item.name}`);
      }
      return null;
    } finally {
      if (!options?.silent) {
        setPreviewLoading(false);
      }
    }
  }, [isAuthenticated, previewCache, token]);

  const handleSearch = (query: string) => {
    console.log("Search:", query);
  };

  const openFilePreview = (item: FileItem) => {
    if (item.isCompressed || !item.previewable) {
      toast.info("Preview unavailable. Download this file to view its contents.");
      return;
    }
    setPreviewFile(item);
    fetchPreview(item);
  };

  const handleItemClick = (item: FileItem) => {
    if (item.type === "folder") {
      setCurrentFolderId(item.id);
      setBreadcrumbStack((prev) => [...prev, { id: item.id, label: item.name }]);
    } else if (item.isCompressed) {
      toast.info("Compressed files skip preview. Downloading instead.");
      downloadItem(item);
    } else {
      openFilePreview(item);
    }
  };

  const handleBreadcrumbNavigate = (index: number) => {
    const target = breadcrumbStack[index];
    if (!target) return;
    setBreadcrumbStack((prev) => prev.slice(0, index + 1));
    setCurrentFolderId(target.id);
  };

  const downloadItem = async (item: FileItem) => {
    if (!isAuthenticated || !token) {
      toast.error("Please login to download items");
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

      toast.success(`Started download for ${item.name}`);
    } catch (error) {
      console.error("Download failed", error);
      toast.error(`Failed to download ${item.name}`);
    }
  };

  const handleItemAction = (action: string, item: FileItem) => {
    switch (action) {
      case "preview":
        if (item.isCompressed) {
          toast.info("Compressed files must be downloaded to view.");
        } else {
          openFilePreview(item);
        }
        break;
      case "rename":
        toast.info("Rename functionality would go here");
        break;
      case "download":
        downloadItem(item);
        break;
      case "delete":
        deleteItem(item);
        break;
      case "favorite":
        addToFavorites(item);
        break;
      case "unfavorite":
        removeFromFavorites(item);
        break;
    }
  };

  const deleteItem = async (item: FileItem) => {
    if (!isAuthenticated || !token) {
      toast.error("Please login to delete items");
      return;
    }

    try {
      // Move to trash instead of hard delete
      const response = await fetch(
        `${API_BASE}/trash`,
        {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`,
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ id: item.id, type: item.type }),
        }
      );

      if (!response.ok) {
        throw new Error('Failed to move item to trash');
      }

      toast.success(`${item.name} moved to trash`);
      fetchContents(currentFolderId); // Refresh the list
    } catch (error) {
      console.error('Error moving item to trash:', error);
      toast.error(`Failed to delete ${item.name}`);
    }
  };

  const addToFavorites = async (item: FileItem) => {
    if (!isAuthenticated || !token) {
      toast.error("Please login to add favorites");
      return;
    }

    try {
      const response = await fetch(`${API_BASE}/favorites`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          id: item.id,
          type: item.type,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to add to favorites');
      }

      toast.success(`${item.name} added to favorites`);
      fetchContents(currentFolderId); // Refresh to update favorite status
    } catch (error) {
      console.error('Error adding to favorites:', error);
      toast.error(`Failed to add ${item.name} to favorites`);
    }
  };

  const removeFromFavorites = async (item: FileItem) => {
    if (!isAuthenticated || !token) {
      toast.error("Please login to remove favorites");
      return;
    }

    try {
      const response = await fetch(`${API_BASE}/favorites`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          id: item.id,
          type: item.type,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to remove from favorites');
      }

      toast.success(`${item.name} removed from favorites`);
      fetchContents(currentFolderId); // Refresh to update favorite status
    } catch (error) {
      console.error('Error removing from favorites:', error);
      toast.error(`Failed to remove ${item.name} from favorites`);
    }
  };

  const handleUploadFile = () => {
    const input = document.createElement("input");
    input.type = "file";
    input.multiple = true;
    input.onchange = (e) => {
      const files = (e.target as HTMLInputElement).files;
      if (files) {
        simulateUpload(files);
      }
    };
    input.click();
  };

  const handleUploadFolder = () => {
    const input = document.createElement("input");
    input.type = "file";
    input.webkitdirectory = true;
    input.multiple = true;
    input.onchange = (e) => {
      const files = (e.target as HTMLInputElement).files;
      if (files && files.length > 0) {
        // Get folder name from the first file's path
        const firstFile = files[0];
        const pathParts = firstFile.webkitRelativePath.split("/");
        const folderName = pathParts[0] || "uploaded_folder";
        
        // Automatically upload with storage mode based on compression:
        // - If compressed: quick storage (single file)
        // - If not compressed: indexed storage (preserve structure)
        uploadFolder(files, folderName);
      }
    };
    input.click();
  };

  const handleNewFolder = async () => {
    if (!isAuthenticated || !token) {
      toast.error("Please login to create folders");
      return;
    }

    const folderName = prompt("Enter folder name:");
    if (!folderName || folderName.trim() === "") {
      return;
    }

    try {
      const response = await fetch(`${API_BASE}/folder`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({
          name: folderName.trim(),
          parent_id: currentFolderId || "",
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to create folder');
      }

      const data = await response.json();
      toast.success(`Folder "${data.name}" created successfully`);
      fetchContents(currentFolderId); // Refresh the list
    } catch (error) {
      console.error('Error creating folder:', error);
      toast.error('Failed to create folder');
    }
  };

  const handleDrop = (files: FileList) => {
    simulateUpload(files);
  };

  const simulateUpload = (files: FileList) => {
    // Check if user is authenticated
    if (!isAuthenticated || !token) {
      toast.error("Please login to upload files");
      return;
    }

    Array.from(files).forEach((file) => {
      const uploadItem: UploadItem = {
        id: Math.random().toString(),
        name: `${file.name}${compressFiles ? "" : " (encrypted only)"}`,
        progress: 0,
        status: "uploading",
        speed: "2.5 MB/s",
      };

      setUploads((prev) => [...prev, uploadItem]);

      // Create FormData for the upload
      const formData = new FormData();
      formData.append("file", file);
      formData.append("compress", compressFiles ? "true" : "false");
      if (currentFolderId) {
        formData.append("folder_id", currentFolderId);
      }

      // Make actual API call to the backend
      const xhr = new XMLHttpRequest();

      // Track upload progress
      xhr.upload.addEventListener("progress", (e) => {
        if (e.lengthComputable) {
          const percentComplete = (e.loaded / e.total) * 100;
          setUploads((prev) =>
            prev.map((item) => {
              if (item.id === uploadItem.id) {
                return {
                  ...item,
                  progress: percentComplete,
                };
              }
              return item;
            })
          );
        }
      });

      // Handle completion
      xhr.addEventListener("load", () => {
        if (xhr.status === 200 || xhr.status === 201) {
          setUploads((prev) =>
            prev.map((item) => {
              if (item.id === uploadItem.id) {
                return {
                  ...item,
                  progress: 100,
                  status: "complete",
                };
              }
              return item;
            })
          );
          toast.success(`${file.name} uploaded successfully`);
          fetchContents(currentFolderId); // Refresh file list
        } else {
          setUploads((prev) =>
            prev.map((item) => {
              if (item.id === uploadItem.id) {
                return {
                  ...item,
                  status: "error",
                };
              }
              return item;
            })
          );
          toast.error(`Failed to upload ${file.name}`);
        }
      });

      // Handle errors
      xhr.addEventListener("error", () => {
        setUploads((prev) =>
          prev.map((item) => {
            if (item.id === uploadItem.id) {
              return {
                ...item,
                status: "error",
              };
            }
            return item;
          })
        );
        toast.error(`Error uploading ${file.name}`);
      });

      // Send the request
      xhr.open("POST", `${API_BASE}/upload`);
      xhr.setRequestHeader("Authorization", `Bearer ${token}`);
      xhr.send(formData);
    });
  };

  const uploadFolder = (files: FileList, folderName: string) => {
    const fileArray = Array.from(files);
    if (fileArray.length === 0) return;

    // Check if user is authenticated
    if (!isAuthenticated || !token) {
      toast.error("Please login to upload folders");
      return;
    }

    // Storage mode is automatically determined:
    // - If compressed: quick storage (single compressed file)
    // - If not compressed: indexed storage (preserve folder/file structure)
    const storageMode = compressFolders ? "Quick Storage" : "Indexed Storage";

    const uploadItem: UploadItem = {
      id: Math.random().toString(),
      name: `${folderName} (${fileArray.length} files) - ${storageMode}${compressFolders ? "" : " - Encryption only"}`,
      progress: 0,
      status: "uploading",
      speed: "calculating...",
    };

    setUploads((prev) => [...prev, uploadItem]);

    // Create FormData for the upload
    const formData = new FormData();
    formData.append("folderName", folderName);
    formData.append("compress", compressFolders ? "true" : "false");
    if (currentFolderId) {
      formData.append("folder_id", currentFolderId);
    }

    // Build metadata manifest so the backend knows each file's relative path
    const entries = fileArray.map((file, index) => ({
      field: `file_${index}`,
      relativePath: file.webkitRelativePath || file.name,
    }));
    formData.append("file_metadata", JSON.stringify(entries));

    // Attach files using their dedicated field names
    entries.forEach((entry, index) => {
      const file = fileArray[index];
      formData.append(entry.field, file);
    });

    // Make actual API call to the backend
    const xhr = new XMLHttpRequest();

    // Track upload progress
    xhr.upload.addEventListener("progress", (e) => {
      if (e.lengthComputable) {
        const percentComplete = (e.loaded / e.total) * 100;
        const speed = calculateSpeed(e.loaded, Date.now());
        setUploads((prev) =>
          prev.map((item) => {
            if (item.id === uploadItem.id) {
              return {
                ...item,
                progress: percentComplete,
                speed,
              };
            }
            return item;
          })
        );
      }
    });

    // Handle completion
    xhr.addEventListener("load", () => {
      if (xhr.status === 200) {
        setUploads((prev) =>
          prev.map((item) => {
            if (item.id === uploadItem.id) {
              return {
                ...item,
                progress: 100,
                status: "complete",
              };
            }
            return item;
          })
        );
        toast.success(
          `${folderName} uploaded successfully (${fileArray.length} files) - ${storageMode}`
        );
        fetchContents(currentFolderId); // Refresh file list
      } else {
        setUploads((prev) =>
          prev.map((item) => {
            if (item.id === uploadItem.id) {
              return {
                ...item,
                status: "error",
              };
            }
            return item;
          })
        );
        toast.error(`Failed to upload ${folderName}`);
      }
    });

    // Handle errors
    xhr.addEventListener("error", () => {
      setUploads((prev) =>
        prev.map((item) => {
          if (item.id === uploadItem.id) {
            return {
              ...item,
              status: "error",
            };
          }
          return item;
        })
      );
      toast.error(`Error uploading ${folderName}`);
    });

    // Send the request
    xhr.open("POST", `${API_BASE}/upload-folder`);
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    xhr.send(formData);
  };

  const calculateSpeed = (loaded: number, startTime: number) => {
    const elapsed = (Date.now() - startTime) / 1000; // seconds
    const speedBps = loaded / elapsed;
    if (speedBps > 1024 * 1024) {
      return `${(speedBps / (1024 * 1024)).toFixed(2)} MB/s`;
    } else if (speedBps > 1024) {
      return `${(speedBps / 1024).toFixed(2)} KB/s`;
    }
    return `${speedBps.toFixed(0)} B/s`;
  };

  const handleCancelUpload = (id: string) => {
    setUploads((prev) => prev.filter((item) => item.id !== id));
    toast.info("Upload cancelled");
  };

  useEffect(() => {
    return () => {
      Object.values(previewCacheRef.current).forEach((data) => {
        if (data?.url) {
          URL.revokeObjectURL(data.url);
        }
      });
    };
  }, []);

  useEffect(() => {
    if (!isAuthenticated || !token) {
      return;
    }

    const previewableImages = files
      .filter((item) => item.type === "file" && !item.isCompressed && item.previewable && item.fileType === "image")
      .slice(0, 12);

    previewableImages.forEach((item) => {
      if (!previewCache[item.id]) {
        fetchPreview(item, { silent: true });
      }
    });
  }, [files, fetchPreview, isAuthenticated, previewCache, token]);

  return (
    <ThemeProvider attribute="class" defaultTheme="light">
      <SidebarProvider>
        <div className="flex min-h-screen w-full">
          <AppSidebar />
          
          <div className="flex-1 flex flex-col">
            <div className="border-b bg-background/60 px-4 py-2">
              <SidebarTrigger />
            </div>
            
            <Navbar
              breadcrumbs={breadcrumbLabels}
              onSearch={handleSearch}
              onBreadcrumbClick={handleBreadcrumbNavigate}
            />

            <DropZone onDrop={handleDrop}>
              <UploadControls
                onUploadFile={handleUploadFile}
                onUploadFolder={handleUploadFolder}
                onNewFolder={handleNewFolder}
              />

              <div className="flex flex-wrap items-center gap-6 px-6 py-3 border-b bg-muted/40 text-sm">
                <div className="flex items-center gap-2">
                  <Switch
                    id="compress-files"
                    checked={compressFiles}
                    onCheckedChange={setCompressFiles}
                  />
                  <Label htmlFor="compress-files" className="cursor-pointer">
                    Compress file uploads before encryption
                  </Label>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    id="compress-folders"
                    checked={compressFolders}
                    onCheckedChange={setCompressFolders}
                  />
                  <Label htmlFor="compress-folders" className="cursor-pointer">
                    Compress folder archives before encryption
                  </Label>
                </div>
                <div className="ml-auto flex items-center gap-2">
                  <span className="text-xs uppercase tracking-wide text-muted-foreground">View</span>
                  <Button
                    variant={viewMode === "grid" ? "default" : "ghost"}
                    size="icon"
                    onClick={() => setViewMode("grid")}
                    aria-label="Grid view"
                  >
                    <LayoutGrid className="h-4 w-4" />
                  </Button>
                  <Button
                    variant={viewMode === "list" ? "default" : "ghost"}
                    size="icon"
                    onClick={() => setViewMode("list")}
                    aria-label="List view"
                  >
                    <ListIcon className="h-4 w-4" />
                  </Button>
                </div>
              </div>

              {viewMode === "grid" ? (
                <FileGrid
                  items={files}
                  onItemClick={handleItemClick}
                  onItemAction={handleItemAction}
                  previewUrls={previewUrls}
                />
              ) : (
                <FileList
                  items={files}
                  onItemClick={handleItemClick}
                  onItemAction={handleItemAction}
                  previewUrls={previewUrls}
                />
              )}
            </DropZone>
          </div>

          <UploadQueue
            uploads={uploads}
            onCancel={handleCancelUpload}
            onClose={() => setUploads([])}
          />

          <FilePreviewModal
            file={previewFile}
            previewData={previewFile ? previewCache[previewFile.id] ?? null : null}
            loading={previewLoading}
            open={!!previewFile}
            onClose={() => setPreviewFile(null)}
            onDownload={() => {
              if (previewFile) {
                downloadItem(previewFile).finally(() => setPreviewFile(null));
              }
            }}
            onDelete={() => {
              if (previewFile) {
                deleteItem(previewFile).finally(() => setPreviewFile(null));
              }
            }}
            onRename={() => {
              toast.info("Rename functionality would go here");
            }}
          />
        </div>
      </SidebarProvider>
    </ThemeProvider>
  );
};

export default Index;
