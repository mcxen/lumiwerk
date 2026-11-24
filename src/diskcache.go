package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const defaultDiskThumbBudget = 512 << 20

// diskThumbs is the persistent thumbnail store behind thumbLRU: encoded
// thumbnail bytes (JPEG/PNG as the pipeline produced them) keyed by path,
// modification time, size and size band, in one bbolt file under the
// user cache folder. A second instance finds the file locked and runs
// memory-only. Values carry their write time for oldest-first eviction.
var diskThumbs = &thumbStore{limit: defaultDiskThumbBudget}

type thumbStore struct {
	mu       sync.RWMutex
	evictMu  sync.Mutex
	opened   bool
	db       *bolt.DB
	path     string
	openPath string
	limit    int64
}

var (
	thumbBucket = []byte("thumbs")
	metaBucket  = []byte("meta")
	totalKey    = []byte("total")
)

func thumbStorePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "lumiwerk", "thumbs.db")
}

func (s *thumbStore) resolvedPathLocked() string {
	if s.openPath != "" {
		return s.openPath
	}
	if s.path != "" {
		return s.path
	}
	return thumbStorePath()
}

func (s *thumbStore) ensureOpen() *bolt.DB {
	s.mu.RLock()
	if s.opened {
		db := s.db
		s.mu.RUnlock()
		return db
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.opened {
		return s.db
	}
	s.opened = true
	if s.limit <= 0 {
		s.limit = defaultDiskThumbBudget
	}
	p := s.path
	if p == "" {
		p = thumbStorePath()
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		logWarnf("thumbnail disk cache: %v; memory only", err)
		return nil
	}
	// bbolt never shrinks its file: a file far past the cap is
	// mostly free pages, so start it over instead of compacting.
	if st, err := os.Stat(p); err == nil && st.Size() > 2*s.limit {
		_ = os.Remove(p)
	}
	db, err := bolt.Open(p, 0o644, &bolt.Options{Timeout: 300 * time.Millisecond, NoFreelistSync: true})
	if err != nil {
		logWarnf("thumbnail disk cache %s unavailable (%v); memory only", p, err)
		return nil
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(thumbBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists(metaBucket)
		return err
	}); err != nil {
		logWarnf("thumbnail disk cache init: %v; memory only", err)
		_ = db.Close()
		return nil
	}
	s.db = db
	s.openPath = p
	return s.db
}

func (s *thumbStore) withDB(fn func(db *bolt.DB)) {
	if s.ensureOpen() == nil {
		return
	}
	s.mu.RLock()
	db := s.db
	if db != nil {
		fn(db)
	}
	s.mu.RUnlock()
}

func thumbStoreKey(path string, edge int) ([]byte, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	return []byte(fmt.Sprintf("%s\x00%d\x00%d\x00%d", path, st.ModTime().UnixNano(), st.Size(), thumbBand(edge))), true
}

// Get returns the stored bytes for path at edge's band, if current.
func (s *thumbStore) Get(path string, edge int) ([]byte, bool) {
	key, ok := thumbStoreKey(path, edge)
	if !ok {
		return nil, false
	}
	var out []byte
	s.withDB(func(db *bolt.DB) {
		_ = db.View(func(tx *bolt.Tx) error {
			if v := tx.Bucket(thumbBucket).Get(key); len(v) > 8 {
				out = append([]byte(nil), v[8:]...)
			}
			return nil
		})
	})
	return out, out != nil
}

// Put stores data and evicts the oldest entries once over the limit.
func (s *thumbStore) Put(path string, edge int, data []byte) {
	if len(data) == 0 {
		return
	}
	key, ok := thumbStoreKey(path, edge)
	if !ok {
		return
	}
	val := make([]byte, 8+len(data))
	binary.BigEndian.PutUint64(val, uint64(time.Now().Unix()))
	copy(val[8:], data)
	var total int64
	var over bool
	limit := s.Limit()
	s.withDB(func(db *bolt.DB) {
		err := db.Update(func(tx *bolt.Tx) error {
			b, m := tx.Bucket(thumbBucket), tx.Bucket(metaBucket)
			total = readTotal(m)
			if old := b.Get(key); old != nil {
				total -= int64(len(old))
			}
			if err := b.Put(key, val); err != nil {
				return err
			}
			total += int64(len(val))
			return writeTotal(m, total)
		})
		if err != nil {
			logWarnf("thumbnail disk cache put %s: %v", path, err)
			return
		}
		over = total > limit
	})
	if over {
		s.evict()
	}
}

// evict drops the oldest entries until the store is at 80% of its limit.
func (s *thumbStore) evict() {
	s.evictMu.Lock()
	defer s.evictMu.Unlock()
	limit := s.Limit()
	s.withDB(func(db *bolt.DB) {
		_ = db.Update(func(tx *bolt.Tx) error {
			b, m := tx.Bucket(thumbBucket), tx.Bucket(metaBucket)
			total := readTotal(m)
			if total <= limit {
				return nil
			}
			type entry struct {
				key  []byte
				at   uint64
				size int
			}
			var all []entry
			_ = b.ForEach(func(k, v []byte) error {
				if len(v) >= 8 {
					all = append(all, entry{append([]byte(nil), k...), binary.BigEndian.Uint64(v), len(v)})
				}
				return nil
			})
			sort.Slice(all, func(i, j int) bool { return all[i].at < all[j].at })
			for _, e := range all {
				if total <= limit*8/10 {
					break
				}
				if err := b.Delete(e.key); err != nil {
					return err
				}
				total -= int64(e.size)
			}
			return writeTotal(m, total)
		})
	})
}

// Limit returns the configured disk thumbnail byte budget.
func (s *thumbStore) Limit() int64 {
	s.mu.RLock()
	limit := s.limit
	s.mu.RUnlock()
	if limit <= 0 {
		return defaultDiskThumbBudget
	}
	return limit
}

// SetLimit updates the disk cache budget and trims excess stored entries.
func (s *thumbStore) SetLimit(limit int64) {
	if limit <= 0 {
		limit = defaultDiskThumbBudget
	}
	s.mu.Lock()
	s.limit = limit
	opened := s.opened && s.db != nil
	s.mu.Unlock()
	if opened && s.Size() > limit {
		s.evict()
	}
}

// Size is the stored logical bytes, 0 when the store is unavailable.
func (s *thumbStore) Size() int64 {
	var total int64
	s.withDB(func(db *bolt.DB) {
		_ = db.View(func(tx *bolt.Tx) error {
			total = readTotal(tx.Bucket(metaBucket))
			return nil
		})
	})
	return total
}

// FileSize returns the physical size of the backing database file in bytes.
func (s *thumbStore) FileSize() int64 {
	s.mu.RLock()
	p := s.resolvedPathLocked()
	s.mu.RUnlock()
	info, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return info.Size()
}

// Clear closes the database and removes its backing file so disk space is
// reclaimed immediately; the next access lazily creates a fresh database.
func (s *thumbStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	p := s.resolvedPathLocked()
	s.opened = false
	s.openPath = ""
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		logWarnf("thumbnail disk cache remove %s: %v", p, err)
	}
}

// Close closes the backing bbolt database if open.
func (s *thumbStore) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		_ = s.db.Close()
		s.db = nil
	}
	s.opened = false
	s.openPath = ""
}

func readTotal(m *bolt.Bucket) int64 {
	if v := m.Get(totalKey); len(v) == 8 {
		return int64(binary.BigEndian.Uint64(v))
	}
	return 0
}

func writeTotal(m *bolt.Bucket, total int64) error {
	var v [8]byte
	binary.BigEndian.PutUint64(v[:], uint64(max(total, 0)))
	return m.Put(totalKey, v[:])
}
