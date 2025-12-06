package services

import "time"

// RecentItem describes a recent file/folder entry returned by FetchRecent.
type RecentItem struct {
    Path      string
    OwnerID   string
    Timestamp time.Time
}

// FetchRecent returns a list of recent items. This is a stub that should
// query the metadata store and return ordered results.
func FetchRecent(limit int) ([]RecentItem, error) {
    if limit <= 0 {
        limit = 50
    }
    // TODO: fetch from DB
    return []RecentItem{}, nil
}
