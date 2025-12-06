import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Archive, Files } from "lucide-react";

interface FolderUploadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChoose: (indexFiles: boolean) => void;
  folderName: string;
  fileCount: number;
}

export const FolderUploadDialog = ({
  open,
  onOpenChange,
  onChoose,
  folderName,
  fileCount,
}: FolderUploadDialogProps) => {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="max-w-2xl">
        <AlertDialogHeader>
          <AlertDialogTitle>Choose Upload Mode for "{folderName}"</AlertDialogTitle>
          <AlertDialogDescription className="text-base">
            You're uploading {fileCount} file{fileCount !== 1 ? "s" : ""}. Choose how you want to store this folder:
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 my-6">
          {/* Quick Storage Option */}
          <button
            onClick={() => {
              onChoose(false);
              onOpenChange(false);
            }}
            className="group relative p-6 rounded-lg border-2 border-border hover:border-primary hover:bg-accent/50 transition-all text-left"
          >
            <div className="flex items-start gap-4">
              <div className="p-3 rounded-lg bg-primary/10 text-primary">
                <Archive className="h-6 w-6" />
              </div>
              <div className="flex-1">
                <h3 className="font-semibold text-lg mb-2">Quick Storage</h3>
                <p className="text-sm text-muted-foreground mb-3">
                  Store as a single compressed archive. Fast upload, but individual files cannot be accessed without downloading the entire folder.
                </p>
                <div className="flex flex-col gap-1 text-xs">
                  <span className="text-green-600 dark:text-green-400">✓ Fastest upload</span>
                  <span className="text-green-600 dark:text-green-400">✓ Maximum compression</span>
                  <span className="text-amber-600 dark:text-amber-400">• Download entire folder to access files</span>
                </div>
              </div>
            </div>
          </button>

          {/* Indexed Storage Option */}
          <button
            onClick={() => {
              onChoose(true);
              onOpenChange(false);
            }}
            className="group relative p-6 rounded-lg border-2 border-border hover:border-primary hover:bg-accent/50 transition-all text-left"
          >
            <div className="flex items-start gap-4">
              <div className="p-3 rounded-lg bg-primary/10 text-primary">
                <Files className="h-6 w-6" />
              </div>
              <div className="flex-1">
                <h3 className="font-semibold text-lg mb-2">Indexed Storage</h3>
                <p className="text-sm text-muted-foreground mb-3">
                  Index all individual files for browsing. You can see and access each file separately without downloading the entire folder.
                </p>
                <div className="flex flex-col gap-1 text-xs">
                  <span className="text-green-600 dark:text-green-400">✓ Browse individual files</span>
                  <span className="text-green-600 dark:text-green-400">✓ Access specific files</span>
                  <span className="text-green-600 dark:text-green-400">✓ View file metadata</span>
                </div>
              </div>
            </div>
          </button>
        </div>

        <AlertDialogFooter>
          <AlertDialogCancel>Cancel Upload</AlertDialogCancel>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
};
