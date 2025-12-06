import { useEffect, useState } from "react";
import { ThemeProvider } from "next-themes";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { Navbar } from "@/components/Navbar";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useAuth } from "@/contexts/AuthContext";
import { useToast } from "@/hooks/use-toast";
import { Trash2, RotateCcw, Folder, FileText, AlertTriangle, Loader2 } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

interface TrashItem {
  id: string;
  name: string;
  type: "file" | "folder";
  size: number;
  mime_type?: string;
  deleted_at: string;
  days_left: number;
  parent_id?: string;
  total_files?: number;
  total_folders?: number;
}

interface TrashResponse {
  items: TrashItem[];
  total_count: number;
  total_size: number;
}

const TrashPage = () => {
  const breadcrumbs = ["Trash"];
  const { token } = useAuth();
  const { toast } = useToast();
  
  const [trashItems, setTrashItems] = useState<TrashItem[]>([]);
  const [totalSize, setTotalSize] = useState(0);
  const [loading, setLoading] = useState(true);
  const [restoring, setRestoring] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [emptyingTrash, setEmptyingTrash] = useState(false);

  const fetchTrash = async () => {
    try {
      setLoading(true);
      const res = await fetch("http://localhost:8081/v1/browser/trash", {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data: TrashResponse = await res.json();
        setTrashItems(data.items || []);
        setTotalSize(data.total_size);
      }
    } catch (error) {
      console.error("Failed to fetch trash:", error);
      toast({
        title: "Error",
        description: "Failed to fetch trash items",
        variant: "destructive",
      });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (token) {
      fetchTrash();
    }
  }, [token]);

  const handleRestore = async (item: TrashItem) => {
    try {
      setRestoring(item.id);
      const res = await fetch("http://localhost:8081/v1/browser/trash/restore", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ id: item.id, type: item.type }),
      });

      if (res.ok) {
        toast({
          title: "Restored",
          description: `${item.name} has been restored`,
        });
        fetchTrash();
      } else {
        const error = await res.text();
        throw new Error(error);
      }
    } catch (error) {
      toast({
        title: "Error",
        description: `Failed to restore: ${error}`,
        variant: "destructive",
      });
    } finally {
      setRestoring(null);
    }
  };

  const handlePermanentDelete = async (item: TrashItem) => {
    try {
      setDeleting(item.id);
      const res = await fetch("http://localhost:8081/v1/browser/trash", {
        method: "DELETE",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ id: item.id, type: item.type }),
      });

      if (res.ok) {
        toast({
          title: "Deleted",
          description: `${item.name} has been permanently deleted`,
        });
        fetchTrash();
      } else {
        const error = await res.text();
        throw new Error(error);
      }
    } catch (error) {
      toast({
        title: "Error",
        description: `Failed to delete: ${error}`,
        variant: "destructive",
      });
    } finally {
      setDeleting(null);
    }
  };

  const handleEmptyTrash = async () => {
    try {
      setEmptyingTrash(true);
      const res = await fetch("http://localhost:8081/v1/browser/trash/empty", {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });

      if (res.ok) {
        const data = await res.json();
        toast({
          title: "Trash Emptied",
          description: `Deleted ${data.deleted_files} files and ${data.deleted_folders} folders`,
        });
        fetchTrash();
      } else {
        const error = await res.text();
        throw new Error(error);
      }
    } catch (error) {
      toast({
        title: "Error",
        description: `Failed to empty trash: ${error}`,
        variant: "destructive",
      });
    } finally {
      setEmptyingTrash(false);
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
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
              <div className="flex items-center justify-between mb-6">
                <div>
                  <h1 className="text-2xl font-semibold flex items-center gap-2">
                    <Trash2 className="h-6 w-6" />
                    Trash
                  </h1>
                  <p className="text-sm text-muted-foreground mt-1">
                    Items in trash will be automatically deleted after 30 days
                  </p>
                </div>
                
                {trashItems.length > 0 && (
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button variant="destructive" disabled={emptyingTrash}>
                        {emptyingTrash ? (
                          <>
                            <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                            Emptying...
                          </>
                        ) : (
                          <>
                            <Trash2 className="h-4 w-4 mr-2" />
                            Empty Trash
                          </>
                        )}
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogTitle>Empty Trash?</AlertDialogTitle>
                        <AlertDialogDescription>
                          This will permanently delete all {trashItems.length} items ({formatBytes(totalSize)}) from trash. 
                          This action cannot be undone.
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction onClick={handleEmptyTrash} className="bg-destructive text-destructive-foreground">
                          Empty Trash
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                )}
              </div>

              {loading ? (
                <Card>
                  <CardContent className="py-10 text-center">
                    <Loader2 className="h-8 w-8 mx-auto animate-spin text-muted-foreground" />
                    <p className="mt-2 text-muted-foreground">Loading trash...</p>
                  </CardContent>
                </Card>
              ) : trashItems.length === 0 ? (
                <Card>
                  <CardContent className="py-16 text-center">
                    <Trash2 className="h-16 w-16 mx-auto text-muted-foreground mb-4" />
                    <h3 className="text-lg font-medium mb-2">Trash is Empty</h3>
                    <p className="text-sm text-muted-foreground">
                      Items you delete will appear here
                    </p>
                  </CardContent>
                </Card>
              ) : (
                <Card>
                  <CardHeader>
                    <CardTitle>
                      {trashItems.length} item{trashItems.length !== 1 ? "s" : ""} in trash
                    </CardTitle>
                    <CardDescription>
                      Total size: {formatBytes(totalSize)}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Name</TableHead>
                          <TableHead>Type</TableHead>
                          <TableHead>Size</TableHead>
                          <TableHead>Deleted</TableHead>
                          <TableHead>Auto-Delete</TableHead>
                          <TableHead className="text-right">Actions</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {trashItems.map((item) => (
                          <TableRow key={item.id}>
                            <TableCell>
                              <div className="flex items-center gap-2">
                                {item.type === "folder" ? (
                                  <Folder className="h-4 w-4 text-blue-500" />
                                ) : (
                                  <FileText className="h-4 w-4 text-gray-500" />
                                )}
                                <span className="font-medium">{item.name}</span>
                              </div>
                            </TableCell>
                            <TableCell>
                              <Badge variant="outline">
                                {item.type === "folder" ? "Folder" : "File"}
                              </Badge>
                            </TableCell>
                            <TableCell>{formatBytes(item.size)}</TableCell>
                            <TableCell className="text-muted-foreground">
                              {formatDate(item.deleted_at)}
                            </TableCell>
                            <TableCell>
                              {item.days_left <= 7 ? (
                                <Badge variant="destructive" className="flex items-center gap-1 w-fit">
                                  <AlertTriangle className="h-3 w-3" />
                                  {item.days_left} days left
                                </Badge>
                              ) : (
                                <span className="text-muted-foreground">
                                  {item.days_left} days left
                                </span>
                              )}
                            </TableCell>
                            <TableCell className="text-right">
                              <div className="flex items-center justify-end gap-2">
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={() => handleRestore(item)}
                                  disabled={restoring === item.id}
                                >
                                  {restoring === item.id ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <>
                                      <RotateCcw className="h-4 w-4 mr-1" />
                                      Restore
                                    </>
                                  )}
                                </Button>
                                <AlertDialog>
                                  <AlertDialogTrigger asChild>
                                    <Button
                                      variant="destructive"
                                      size="sm"
                                      disabled={deleting === item.id}
                                    >
                                      {deleting === item.id ? (
                                        <Loader2 className="h-4 w-4 animate-spin" />
                                      ) : (
                                        <>
                                          <Trash2 className="h-4 w-4 mr-1" />
                                          Delete
                                        </>
                                      )}
                                    </Button>
                                  </AlertDialogTrigger>
                                  <AlertDialogContent>
                                    <AlertDialogHeader>
                                      <AlertDialogTitle>Delete Permanently?</AlertDialogTitle>
                                      <AlertDialogDescription>
                                        Are you sure you want to permanently delete "{item.name}"? 
                                        This action cannot be undone.
                                      </AlertDialogDescription>
                                    </AlertDialogHeader>
                                    <AlertDialogFooter>
                                      <AlertDialogCancel>Cancel</AlertDialogCancel>
                                      <AlertDialogAction
                                        onClick={() => handlePermanentDelete(item)}
                                        className="bg-destructive text-destructive-foreground"
                                      >
                                        Delete Permanently
                                      </AlertDialogAction>
                                    </AlertDialogFooter>
                                  </AlertDialogContent>
                                </AlertDialog>
                              </div>
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              )}
            </main>
          </div>
        </div>
      </SidebarProvider>
    </ThemeProvider>
  );
};

export default TrashPage;
