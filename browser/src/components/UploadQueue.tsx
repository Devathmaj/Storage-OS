import { X, CheckCircle, AlertCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { Card } from "@/components/ui/card";

export interface UploadItem {
  id: string;
  name: string;
  progress: number;
  status: "uploading" | "complete" | "error";
  speed?: string;
}

interface UploadQueueProps {
  uploads: UploadItem[];
  onCancel: (id: string) => void;
  onClose: () => void;
}

export const UploadQueue = ({ uploads, onCancel, onClose }: UploadQueueProps) => {
  if (uploads.length === 0) return null;

  return (
    <Card className="fixed bottom-6 right-6 w-80 shadow-soft-lg border">
      <div className="p-4 space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="font-semibold">Uploads</h3>
          <Button variant="ghost" size="icon" onClick={onClose} className="h-6 w-6">
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="space-y-3 max-h-96 overflow-y-auto">
          {uploads.map((upload) => (
            <div key={upload.id} className="space-y-2">
              <div className="flex items-start justify-between gap-2">
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium truncate">{upload.name}</p>
                  {upload.speed && upload.status === "uploading" && (
                    <p className="text-xs text-muted-foreground">{upload.speed}</p>
                  )}
                </div>
                <div className="flex items-center gap-1">
                  {upload.status === "complete" && (
                    <CheckCircle className="h-4 w-4 text-green-500" />
                  )}
                  {upload.status === "error" && (
                    <AlertCircle className="h-4 w-4 text-destructive" />
                  )}
                  {upload.status === "uploading" && (
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => onCancel(upload.id)}
                      className="h-6 w-6"
                    >
                      <X className="h-3 w-3" />
                    </Button>
                  )}
                </div>
              </div>
              {upload.status === "uploading" && (
                <Progress value={upload.progress} className="h-1" />
              )}
            </div>
          ))}
        </div>
      </div>
    </Card>
  );
};
