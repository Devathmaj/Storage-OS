import { Upload } from "lucide-react";
import { useState } from "react";

interface DropZoneProps {
  onDrop: (files: FileList) => void;
  children: React.ReactNode;
}

export const DropZone = ({ onDrop, children }: DropZoneProps) => {
  const [isDragging, setIsDragging] = useState(false);

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      onDrop(e.dataTransfer.files);
    }
  };

  return (
    <div
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className="relative min-h-screen"
    >
      {children}

      {isDragging && (
        <div className="fixed inset-0 z-50 bg-primary/10 backdrop-blur-sm flex items-center justify-center">
          <div className="bg-card border-2 border-primary border-dashed rounded-3xl p-12 shadow-soft-lg">
            <div className="flex flex-col items-center gap-4 text-center">
              <div className="rounded-full bg-primary/10 p-6">
                <Upload className="h-12 w-12 text-primary" />
              </div>
              <div>
                <h3 className="text-xl font-semibold mb-2">Drop files here</h3>
                <p className="text-muted-foreground">
                  Release to upload files and folders
                </p>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
