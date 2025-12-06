import { useMemo } from "react";
import { FileItem } from "./FileGrid";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { MoreHorizontal, Folder, FileText, Image, Video, Music, File as FileIcon, Package, Shield } from "lucide-react";

interface FileListProps {
  items: FileItem[];
  onItemClick: (item: FileItem) => void;
  onItemAction: (action: string, item: FileItem) => void;
  previewUrls?: Record<string, string | undefined>;
}

export const FileList = ({ items, onItemClick, onItemAction, previewUrls }: FileListProps) => {
  const sorted = useMemo(() => {
    const folders = items.filter((item) => item.type === "folder");
    const files = items.filter((item) => item.type === "file");
    return [...folders, ...files];
  }, [items]);

  const renderIcon = (item: FileItem) => {
    if (item.type === "folder") {
      return <Folder className="h-5 w-5 text-folder" />;
    }
    if (item.isCompressed) {
      return <Package className="h-5 w-5 text-muted-foreground" />;
    }
    switch (item.fileType) {
      case "image":
        return previewUrls?.[item.id] ? (
          <img src={previewUrls[item.id]} alt={item.name} className="h-9 w-9 rounded object-cover" />
        ) : (
          <Image className="h-5 w-5 text-file" />
        );
      case "video":
        return <Video className="h-5 w-5 text-file" />;
      case "audio":
        return <Music className="h-5 w-5 text-file" />;
      case "text":
      case "document":
        return <FileText className="h-5 w-5 text-file" />;
      default:
        return <FileIcon className="h-5 w-5 text-file" />;
    }
  };

  return (
    <div className="px-4 pb-6">
      <div className="overflow-hidden rounded-xl border bg-card">
        <div className="grid grid-cols-[auto,1fr,150px,150px,80px] px-4 py-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          <span>Name</span>
          <span className="hidden sm:block" />
          <span className="text-right">Size</span>
          <span className="text-right">Modified</span>
          <span className="text-right">Actions</span>
        </div>
        {sorted.map((item) => (
          <div
            key={item.id}
            className="grid grid-cols-[auto,1fr,150px,150px,80px] items-center gap-3 border-t px-4 py-3 text-sm hover:bg-muted/50"
            onDoubleClick={() => onItemClick(item)}
          >
            <div className="flex items-center gap-3 min-w-0">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-muted/60">
                {renderIcon(item)}
              </div>
              <div className="min-w-0">
                <p className="font-medium filename-clamp text-sm leading-snug" title={item.name}>{item.name}</p>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  {item.fileType && item.type === "file" && <span className="capitalize">{item.fileType}</span>}
                  {item.isCompressed && (
                    <span className="inline-flex items-center gap-1 text-primary">
                      <Shield className="h-3 w-3" />
                      Compressed
                    </span>
                  )}
                </div>
              </div>
            </div>
            <div className="hidden sm:flex flex-col text-xs text-muted-foreground">
              {item.type === "folder" ? "Folder" : item.mimeType || item.fileType || "File"}
            </div>
            <div className="text-right text-sm text-muted-foreground">{item.size || "-"}</div>
            <div className="text-right text-sm text-muted-foreground">{item.modified}</div>
            <div className="flex justify-end">
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" className="h-8 w-8">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onClick={() => onItemAction("preview", item)} disabled={item.isCompressed}>
                    Preview
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => onItemAction("download", item)}>
                    Download
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => onItemAction("delete", item)} className="text-destructive">
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
