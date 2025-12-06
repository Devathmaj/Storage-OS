import { X, Download, Trash2, Edit } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { FileItem } from "./FileGrid";

export interface PreviewData {
  url?: string;
  text?: string;
  mimeType?: string;
}

interface FilePreviewModalProps {
  file: FileItem | null;
  previewData: PreviewData | null;
  loading: boolean;
  open: boolean;
  onClose: () => void;
  onDownload: () => void;
  onDelete: () => void;
  onRename: () => void;
}

export const FilePreviewModal = ({
	file,
	previewData,
	loading,
	open,
	onClose,
	onDownload,
	onDelete,
	onRename,
}: FilePreviewModalProps) => {
  if (!file) return null;

  const renderPreview = () => {
    if (file.isCompressed) {
      return (
        <div className="flex min-h-[320px] items-center justify-center rounded-lg bg-muted/30 text-muted-foreground">
          Download the archive to inspect its contents.
        </div>
      );
    }

    if (loading) {
      return (
        <div className="flex min-h-[320px] items-center justify-center rounded-lg bg-muted/30 text-muted-foreground">
          Preparing preview...
        </div>
      );
    }

    if (!previewData) {
      return (
        <div className="flex min-h-[320px] items-center justify-center rounded-lg bg-muted/30 text-muted-foreground">
          Preview not available. Try downloading instead.
        </div>
      );
    }

    if (file.fileType === "image" && previewData.url) {
      return (
        <div className="rounded-lg bg-muted/30 p-4">
          <img src={previewData.url} alt={file.name} className="mx-auto max-h-[480px] w-full rounded-lg object-contain" />
        </div>
      );
    }

    if (file.fileType === "video" && previewData.url) {
      return (
        <video controls className="w-full rounded-lg bg-black" src={previewData.url} />
      );
    }

    if (file.fileType === "audio" && previewData.url) {
      return (
        <audio controls className="w-full">
          <source src={previewData.url} type={previewData.mimeType} />
        </audio>
      );
    }

    if (file.fileType === "text" && previewData.text) {
      return (
        <div className="rounded-lg bg-muted/30 p-4">
          <pre className="max-h-[420px] overflow-y-auto whitespace-pre-wrap text-sm">
            {previewData.text}
          </pre>
        </div>
      );
    }

    if (previewData.url && previewData.mimeType === "application/pdf") {
      return (
        <iframe src={previewData.url} title={file.name} className="h-[420px] w-full rounded-lg bg-white" />
      );
    }

    if (previewData.url) {
      return (
        <div className="flex min-h-[320px] items-center justify-center rounded-lg bg-muted/30 text-muted-foreground">
          <a href={previewData.url} target="_blank" rel="noreferrer" className="text-primary underline">
            Open preview in new tab
          </a>
        </div>
      );
    }

    return (
      <div className="flex min-h-[320px] items-center justify-center rounded-lg bg-muted/30 text-muted-foreground">
        Preview not available
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center justify-between pr-8">
            <span className="truncate">{file.name}</span>
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {renderPreview()}

          <div className="flex items-center justify-between pt-4 border-t">
            <div className="text-sm text-muted-foreground space-y-1">
              <div>Size: {file.size || "Unknown"}</div>
              <div>Modified: {file.modified}</div>
              <div>Type: {file.fileType || "Unknown"}</div>
            </div>

            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" onClick={onRename} className="gap-2">
                <Edit className="h-4 w-4" />
                Rename
              </Button>
              <Button variant="outline" size="sm" onClick={onDownload} className="gap-2">
                <Download className="h-4 w-4" />
                Download
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={onDelete}
                className="gap-2"
              >
                <Trash2 className="h-4 w-4" />
                Delete
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
};
