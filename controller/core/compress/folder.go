package compress

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
)

// CompressFolder compresses an entire folder into a tar.zst archive
// srcFolder: path to the folder to compress
// dstPath: destination path for the compressed tar.zst file
// level: compression level (0 for default, or 1-22 for zstd levels)
func CompressFolder(srcFolder, dstPath string, level int) error {
	// Create output file
	outFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Create zstd encoder
	var zstdWriter *zstd.Encoder
	if level == 0 {
		zstdWriter, err = zstd.NewWriter(outFile)
	} else {
		zstdWriter, err = zstd.NewWriter(outFile, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	}
	if err != nil {
		return fmt.Errorf("failed to create zstd writer: %w", err)
	}
	defer zstdWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(zstdWriter)
	defer tarWriter.Close()

	// Walk through the folder and add files to tar
	err = filepath.Walk(srcFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(srcFolder, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Skip the root folder itself
		if relPath == "." {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}

		// Use relative path as name
		header.Name = relPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("failed to write file %s to tar: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}

	// Close tar writer to flush
	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Close zstd writer to flush
	if err := zstdWriter.Close(); err != nil {
		return fmt.Errorf("failed to close zstd writer: %w", err)
	}

	return nil
}

// ArchiveFolder walks a folder structure and writes it into an uncompressed TAR archive.
// This is useful when callers want to preserve directory structure without paying the
// cost of compression prior to encryption.
func ArchiveFolder(srcFolder, dstPath string) error {
	// Create output file
	outFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	tarWriter := tar.NewWriter(outFile)
	defer tarWriter.Close()

	err = filepath.Walk(srcFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcFolder, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			if _, err := io.Copy(tarWriter, file); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file %s to tar: %w", path, err)
			}
			file.Close()
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	return nil
}

// DecompressFolder decompresses a tar.zst archive into a destination folder
// srcPath: path to the tar.zst file
// dstFolder: destination folder to extract files to
func DecompressFolder(srcPath, dstFolder string) error {
	// Open input file
	inFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer inFile.Close()

	// Create zstd decoder
	zstdReader, err := zstd.NewReader(inFile)
	if err != nil {
		return fmt.Errorf("failed to create zstd reader: %w", err)
	}
	defer zstdReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(zstdReader)

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Construct target path
		target := filepath.Join(dstFolder, header.Name)

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Handle files and directories
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
		case tar.TypeReg:
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
			outFile.Close()
		default:
			// Skip other types (symlinks, etc.)
		}
	}

	return nil
}
