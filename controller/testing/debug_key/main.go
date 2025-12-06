package main

import (
    "database/sql"
    "flag"
    "fmt"
    "log"
    "strings"

    _ "github.com/mattn/go-sqlite3"
    "golang.org/x/crypto/curve25519"
    "storageos/controller/core/crypto"
    "storageos/controller/models"
)

func main() {
    fileID := flag.String("id", "", "file ID to inspect")
    flag.Parse()
    if *fileID == "" {
        log.Fatal("-id is required")
    }

    db, err := sql.Open("sqlite3", "storageos.db")
    if err != nil {
        log.Fatalf("open db: %v", err)
    }
    defer db.Close()

    file, err := models.GetFileByID(db, *fileID)
    if err != nil {
        log.Fatalf("load file: %v", err)
    }

    fmt.Printf("filename=%s relative=%s size=%d\n", file.Filename, file.RelativePath, len(file.EncryptedKey))

    aesKey, info, err := deriveFileAESKey(file)
    if err != nil {
        log.Fatalf("derive key: %v", err)
    }

    fmt.Printf("info=%s aes_len=%d\n", info, len(aesKey))
}

func deriveFileAESKey(file *models.File) ([]byte, []byte, error) {
    if len(file.EncryptedKey) == 0 || len(file.EphemeralPubKey) == 0 {
        return nil, nil, fmt.Errorf("missing encryption metadata for file %s", file.ID)
    }
    receiverPriv := crypto.GetHardcodedPrivateKey()
    calcPub, err := curve25519.X25519(receiverPriv, curve25519.Basepoint)
    if err != nil {
        return nil, nil, fmt.Errorf("derive pub: %w", err)
    }
    log.Printf("derived pub=%x", calcPub)
    log.Printf("stored  pub=%x", crypto.GetHardcodedPublicKey())
    shared, err := crypto.DeriveSharedSecret(receiverPriv, file.EphemeralPubKey)
    if err != nil {
        return nil, nil, fmt.Errorf("derive shared secret: %w", err)
    }
    info := determineKeyInfo(file)
    kek, err := crypto.DeriveAES256KeyFromShared(shared, info)
    if err != nil {
        return nil, nil, fmt.Errorf("derive key encryption key: %w", err)
    }
    aesKey, err := crypto.DecryptData(file.EncryptedKey, kek)
    if err != nil {
        return nil, nil, fmt.Errorf("decrypt AES key: %w", err)
    }
    return aesKey, info, nil
}

func determineKeyInfo(file *models.File) []byte {
    if file.RelativePath != "" || strings.HasSuffix(strings.ToLower(file.Filename), ".tar.zst.enc") {
        return []byte("folder-key-encryption-v1")
    }
    return []byte("file-key-encryption-v1")
}
