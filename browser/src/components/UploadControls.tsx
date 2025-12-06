import { Upload, FolderUp, FolderPlus } from "lucide-react";
import { Button } from "@/components/ui/button";

interface UploadControlsProps {
  onUploadFile: () => void;
  onUploadFolder: () => void;
  onNewFolder: () => void;
}

export const UploadControls = ({
  onUploadFile,
  onUploadFolder,
  onNewFolder,
}: UploadControlsProps) => {
  return (
    <div className="flex items-center gap-2 px-6 py-4 border-b bg-background/60">
      <Button onClick={onUploadFile} className="gap-2">
        <Upload className="h-4 w-4" />
        <span className="hidden sm:inline">Upload File</span>
      </Button>
      <Button onClick={onUploadFolder} variant="outline" className="gap-2">
        <FolderUp className="h-4 w-4" />
        <span className="hidden sm:inline">Upload Folder</span>
      </Button>
      <Button onClick={onNewFolder} variant="outline" className="gap-2">
        <FolderPlus className="h-4 w-4" />
        <span className="hidden sm:inline">New Folder</span>
      </Button>
    </div>
  );
};
