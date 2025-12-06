package compress

import (
    "bufio"
    "bytes"
    "io"
    "os"

    "github.com/klauspost/compress/zstd"
)

// This file provides simple Zstandard compression helpers for files and bytes.
// It uses github.com/klauspost/compress/zstd which is a performant pure-Go implementation.

// CompressFile compresses srcPath into dstPath using Zstandard. If level is 0
// the default encoder settings are used. Positive integers map to zstd levels
// via zstd.EncoderLevelFromZstd(level).
func CompressFile(srcPath, dstPath string, level int) error {
    in, err := os.Open(srcPath)
    if err != nil {
        return err
    }
    defer in.Close()

    out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
    if err != nil {
        return err
    }
    defer out.Close()

    bw := bufio.NewWriter(out)
    defer bw.Flush()

    var enc *zstd.Encoder
    if level == 0 {
        enc, err = zstd.NewWriter(bw)
    } else {
        enc, err = zstd.NewWriter(bw, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
    }
    if err != nil {
        return err
    }
    defer enc.Close()

    br := bufio.NewReader(in)
    if _, err := io.Copy(enc, br); err != nil {
        return err
    }

    // ensure encoder flushes
    return enc.Close()
}

// DecompressFile decompresses a Zstandard-compressed file at srcPath and
// writes the plaintext to dstPath.
func DecompressFile(srcPath, dstPath string) error {
    in, err := os.Open(srcPath)
    if err != nil {
        return err
    }
    defer in.Close()

    dec, err := zstd.NewReader(in)
    if err != nil {
        return err
    }
    defer dec.Close()

    out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
    if err != nil {
        return err
    }
    defer out.Close()

    bw := bufio.NewWriter(out)
    defer bw.Flush()

    if _, err := io.Copy(bw, dec); err != nil {
        return err
    }
    return bw.Flush()
}

// CompressBytes compresses a byte slice and returns the compressed bytes.
// level follows the same convention as CompressFile.
func CompressBytes(src []byte, level int) ([]byte, error) {
    var buf bytes.Buffer
    var enc *zstd.Encoder
    var err error
    if level == 0 {
        enc, err = zstd.NewWriter(&buf)
    } else {
        enc, err = zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
    }
    if err != nil {
        return nil, err
    }
    if _, err := enc.Write(src); err != nil {
        enc.Close()
        return nil, err
    }
    if err := enc.Close(); err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// DecompressBytes decompresses a Zstandard-compressed byte slice.
func DecompressBytes(src []byte) ([]byte, error) {
    dec, err := zstd.NewReader(bytes.NewReader(src))
    if err != nil {
        return nil, err
    }
    defer dec.Close()
    var out bytes.Buffer
    if _, err := io.Copy(&out, dec); err != nil {
        return nil, err
    }
    return out.Bytes(), nil
}
