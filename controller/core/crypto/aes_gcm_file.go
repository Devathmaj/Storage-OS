package crypto

import (
    "bufio"
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/binary"
    "errors"
    "io"
    "os"
)

// This file provides AES-256-GCM file-level encryption and decryption.
// It encrypts files in fixed-size chunks. Each chunk is encrypted with
// a fresh random 12-byte nonce. File format:
// [8 bytes magic][4 bytes chunkSize][chunk...]
// where each chunk is: [12 bytes nonce][4 bytes ciphertextLen][ciphertext]

var (
    // ErrInvalidKey is returned when the provided key isn't 32 bytes.
    ErrInvalidKey = errors.New("invalid key length: must be 32 bytes for AES-256")
)

const (
    magic      = "A256GCM1" // 8 bytes
    nonceSize  = 12
    headerSize = 8 + 4
    // defaultChunkSize determines how much plaintext is read per encryption operation.
    defaultChunkSize = 64 * 1024 // 64 KiB
)

// EncryptFile encrypts the file at srcPath and writes the encrypted output
// to dstPath using AES-256-GCM. The key must be 32 bytes long.
// The function streams the file in chunks so it works with large files.
func EncryptFile(srcPath, dstPath string, key []byte) error {
    if len(key) != 32 {
        return ErrInvalidKey
    }

    in, err := os.Open(srcPath)
    if err != nil {
        return err
    }
    defer in.Close()

    out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
    if err != nil {
        return err
    }
    defer func() {
        _ = out.Sync()
        _ = out.Close()
    }()

    bw := bufio.NewWriter(out)
    defer bw.Flush()

    block, err := aes.NewCipher(key)
    if err != nil {
        return err
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return err
    }

    // write header: magic (8 bytes) + chunk size (uint32 big-endian)
    if _, err := bw.Write([]byte(magic)); err != nil {
        return err
    }
    if err := binary.Write(bw, binary.BigEndian, uint32(defaultChunkSize)); err != nil {
        return err
    }

    buf := make([]byte, defaultChunkSize)
    for {
        n, rerr := io.ReadFull(in, buf)
        if rerr == io.ErrUnexpectedEOF || rerr == io.EOF {
            if n == 0 {
                break
            }
            // proceed with n bytes
        } else if rerr != nil && rerr != io.ErrUnexpectedEOF {
            return rerr
        }

        // generate random nonce
        nonce := make([]byte, nonceSize)
        if _, err := rand.Read(nonce); err != nil {
            return err
        }

        ciphertext := aead.Seal(nil, nonce, buf[:n], nil)

        // write nonce
        if _, err := bw.Write(nonce); err != nil {
            return err
        }
        // write ciphertext length
        if err := binary.Write(bw, binary.BigEndian, uint32(len(ciphertext))); err != nil {
            return err
        }
        // write ciphertext
        if _, err := bw.Write(ciphertext); err != nil {
            return err
        }

        if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
            break
        }
    }

    return bw.Flush()
}

// EncryptData encrypts data in memory using AES-256-GCM and returns the encrypted bytes.
// This is useful for encrypting small amounts of data like keys.
func EncryptData(data, key []byte) ([]byte, error) {
    if len(key) != 32 {
        return nil, ErrInvalidKey
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    // generate random nonce
    nonce := make([]byte, nonceSize)
    if _, err := rand.Read(nonce); err != nil {
        return nil, err
    }

    ciphertext := aead.Seal(nil, nonce, data, nil)

    // Format: [magic][nonce][ciphertext]
    result := make([]byte, 0, 8+nonceSize+len(ciphertext))
    result = append(result, []byte(magic)...)
    result = append(result, nonce...)
    result = append(result, ciphertext...)

    return result, nil
}

// DecryptData decrypts data encrypted with EncryptData.
func DecryptData(encryptedData, key []byte) ([]byte, error) {
    if len(key) != 32 {
        return nil, ErrInvalidKey
    }

    if len(encryptedData) < 8+nonceSize {
        return nil, errors.New("encrypted data too short")
    }

    if string(encryptedData[:8]) != magic {
        return nil, errors.New("invalid encrypted data format: magic mismatch")
    }

    nonce := encryptedData[8:8+nonceSize]
    ciphertext := encryptedData[8+nonceSize:]

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, err
    }

    return plaintext, nil
}

// DecryptFile reads an encrypted file produced by EncryptFile and writes the
// decrypted plaintext to dstPath. The key must be 32 bytes long.
func DecryptFile(srcPath, dstPath string, key []byte) error {
    if len(key) != 32 {
        return ErrInvalidKey
    }

    in, err := os.Open(srcPath)
    if err != nil {
        return err
    }
    defer in.Close()

    out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
    if err != nil {
        return err
    }
    defer func() {
        _ = out.Sync()
        _ = out.Close()
    }()

    // read header
    hdr := make([]byte, headerSize)
    if _, err := io.ReadFull(in, hdr); err != nil {
        return err
    }
    if string(hdr[:8]) != magic {
        return errors.New("invalid file format: magic mismatch")
    }
    chunkSize := binary.BigEndian.Uint32(hdr[8:12])
    if chunkSize == 0 || chunkSize > 10*1024*1024 {
        return errors.New("invalid chunk size in header")
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return err
    }
    aead, err := cipher.NewGCM(block)
    if err != nil {
        return err
    }

    // loop reading nonce + length + ciphertext
    for {
        nonce := make([]byte, nonceSize)
        if _, err := io.ReadFull(in, nonce); err != nil {
            if err == io.EOF || err == io.ErrUnexpectedEOF {
                break
            }
            return err
        }

        var clen uint32
        if err := binary.Read(in, binary.BigEndian, &clen); err != nil {
            return err
        }
        if clen == 0 || clen > uint32(chunkSize)+uint32(aead.Overhead())+1024 {
            return errors.New("invalid ciphertext length")
        }

        cbuf := make([]byte, clen)
        if _, err := io.ReadFull(in, cbuf); err != nil {
            return err
        }

        plain, err := aead.Open(nil, nonce, cbuf, nil)
        if err != nil {
            return err
        }

        if _, err := out.Write(plain); err != nil {
            return err
        }
    }

    return nil
}
