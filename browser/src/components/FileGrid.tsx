import { useMemo, useState } from "react";
import { Folder, File, Image, FileText, Video, Music, MoreVertical, Package, Shield, Star } from "lucide-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";

export interface FileItem {
  id: string;
  name: string;
  type: "folder" | "file";
  fileType?: "image" | "video" | "text" | "audio" | "document" | "archive" | "file";
  size?: string;
  modified: string;
  extension?: string;
  nodeId?: string;
  isCompressed?: boolean;
  mimeType?: string;
  previewable?: boolean;
  isFavorite?: boolean;
}

interface FileGridProps {
  items: FileItem[];
  onItemClick: (item: FileItem) => void;
  onItemAction: (action: string, item: FileItem) => void;
  previewUrls?: Record<string, string | undefined>;
}

export const FileGrid = ({ items, onItemClick, onItemAction, previewUrls }: FileGridProps) => {
  const [selectedItems, setSelectedItems] = useState<string[]>([]);

  const sortedItems = useMemo(() => {
    const folders = items.filter((item) => item.type === "folder");
    const files = items.filter((item) => item.type === "file");
    return [...folders, ...files];
  }, [items]);

  const renderThumbnail = (item: FileItem) => {
    if (item.type === "folder") {
      return (
        <div className="flex h-24 items-center justify-center rounded-xl bg-primary/10">
          <Folder className="h-12 w-12 text-folder" />
        </div>
      );
    }

    if (item.isCompressed) {
      return (
        <div className="flex h-24 items-center justify-center rounded-xl bg-muted">
          <Package className="h-10 w-10 text-muted-foreground" />
        </div>
      );
    }

    if (item.fileType === "image" && previewUrls?.[item.id]) {
      return (
        <div className="h-24 w-full overflow-hidden rounded-xl bg-muted">
          <img
            src={previewUrls[item.id]}
            alt={item.name}
            className="h-full w-full object-cover"
            loading="lazy"
          />
        </div>
      );
    }

    const baseIconProps = "h-12 w-12 text-file";
    switch (item.fileType) {
      case "image":
        return <Image className={baseIconProps} />;
      case "video":
        return <Video className={baseIconProps} />;
      case "text":
      case "document":
        return <FileText className={baseIconProps} />;
      case "audio":
        return <Music className={baseIconProps} />;
      default:
        return <File className={baseIconProps} />;
    }
  };

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 p-6">
      {sortedItems.map((item) => (
        <ContextMenu key={item.id}>
          <ContextMenuTrigger>
            <div
              className={`group relative bg-card rounded-2xl p-4 cursor-pointer hover-lift border ${
                selectedItems.includes(item.id)
                  ? "ring-2 ring-primary"
                  : "border-transparent hover:border-border"
              }`}
              onDoubleClick={() => onItemClick(item)}
              onClick={(e) => {
                if (e.ctrlKey || e.metaKey) {
                  setSelectedItems((prev) =>
                    prev.includes(item.id)
                      ? prev.filter((id) => id !== item.id)
                      : [...prev, item.id]
                  );
                } else {
                  setSelectedItems([item.id]);
                }
              }}
            >
              <div className="flex flex-col items-center gap-3">
                <div className="w-full transition-transform group-hover:scale-[1.02] duration-300">
                  {renderThumbnail(item)}
                </div>
                <div className="w-full text-center space-y-1">
                  <div className="flex flex-col items-center gap-1">
                    <p className="text-sm font-medium filename-clamp text-center" title={item.name}>
                      {item.name}
                    </p>
                    {item.isCompressed && (
                      <span className="flex items-center gap-1 text-[10px] uppercase tracking-wide text-primary">
                        <Shield className="h-3 w-3" />
                        Compressed
                      </span>
                    )}
                  </div>
                  <div className="flex items-center justify-center gap-1 text-xs text-muted-foreground">
                    {item.size && <span>{item.size}</span>}
                  </div>
                </div>
              </div>

              <Button
                variant="ghost"
                size="icon"
                className="absolute top-2 left-2 h-7 w-7 opacity-0 group-hover:opacity-100 transition-opacity"
                onClick={(e) => {
                  e.stopPropagation();
                  onItemAction(item.isFavorite ? "unfavorite" : "favorite", item);
                }}
              >
                <Star className={`h-4 w-4 ${item.isFavorite ? "fill-yellow-400 text-yellow-400" : ""}`} />
              </Button>

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 transition-opacity h-7 w-7"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <MoreVertical className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={() => onItemAction(item.isFavorite ? "unfavorite" : "favorite", item)}
                  >
                    {item.isFavorite ? "Remove from Favorites" : "Add to Favorites"}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => onItemAction("preview", item)}
                    disabled={item.isCompressed}
                  >
                    Preview
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => onItemAction("rename", item)}>
                    Rename
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => onItemAction("download", item)}>
                    Download
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => onItemAction("delete", item)}
                    className="text-destructive"
                  >
                    Delete
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          </ContextMenuTrigger>
          <ContextMenuContent>
            <ContextMenuItem
              onClick={() => onItemAction(item.isFavorite ? "unfavorite" : "favorite", item)}
            >
              {item.isFavorite ? "Remove from Favorites" : "Add to Favorites"}
            </ContextMenuItem>
            <ContextMenuItem
              onClick={() => onItemAction("preview", item)}
              disabled={item.isCompressed}
            >
              Preview
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onItemAction("rename", item)}>
              Rename
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onItemAction("download", item)}>
              Download
            </ContextMenuItem>
            <ContextMenuItem
              onClick={() => onItemAction("delete", item)}
              className="text-destructive"
            >
              Delete
            </ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>
      ))}
    </div>
  );
};
