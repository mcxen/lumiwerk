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

// diskThumbs is the persistent thumbnail store behind thumbLRU: encoded
// thumbnail bytes (JPEG/PNG as the pipeline produced them) keyed by path,
// modification time, size and size band, in one bbolt file under the
// user cache folder. A second instance finds the file locked and runs
// memory-only. Values carry their write time for oldest-first eviction.
var diskThumbs = &thumbStore{limit: 512 << 20}

type thumbStore struct {
	once   sync.Once
	mu     sync.Mutex
	db     *bolt.DB
	path   string
	limit  int64
	writes int
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

func (s *thumbStore) open() *bolt.DB {
	s.once.Do(func() {
		if s.path == "" {
			s.path = thumbStorePath()
		}
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			logWarnf("thumbnail disk cache: %v; memory only", err)
			return
		}
		// bbolt never shrinks its file: a file far past the cap is
		// mostly free pages, so start it over instead of compacting.
		if st, err := os.Stat(s.path); err == nil && st.Size() > 2*s.limit {
			_ = os.Remove(s.path)
		}
		db, err := bolt.Open(s.path, 0o644, &bolt.Options{Timeout: 300 * time.Millisecond, NoFreelistSync: true})
		if err != nil {
			logWarnf("thumbnail disk cache %s unavailable (%v); memory only", s.path, err)
			return
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
			return
		}
		s.db = db
	})
	return s.db
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
	db := s.open()
	if db == nil {
		return nil, false
	}
	key, ok := thumbStoreKey(path, edge)
	if !ok {
		return nil, false
	}
	var out []byte
	_ = db.View(func(tx *bolt.Tx) error {
		if v := tx.Bucket(thumbBucket).Get(key); len(v) > 8 {
			out = append([]byte(nil), v[8:]...)
		}
		return nil
	})
	return out, out != nil
}

// Put stores data and evicts the oldest entries once over the limit.
func (s *thumbStore) Put(path string, edge int, data []byte) {
	db := s.open()
	if db == nil || len(data) == 0 {
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
	if total > s.limit {
		s.evict(db)
	}
}

// evict drops the oldest entries until the store is at 80% of its limit.
func (s *thumbStore) evict(db *bolt.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = db.Update(func(tx *bolt.Tx) error {
		b, m := tx.Bucket(thumbBucket), tx.Bucket(metaBucket)
		total := readTotal(m)
		if total <= s.limit {
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
			if total <= s.limit*8/10 {
				break
			}
			if err := b.Delete(e.key); err != nil {
				return err
			}
			total -= int64(e.size)
		}
		return writeTotal(m, total)
	})
}

// Size is the stored bytes, 0 when the store is unavailable.
func (s *thumbStore) Size() int64 {
	db := s.open()
	if db == nil {
		return 0
	}
	var total int64
	_ = db.View(func(tx *bolt.Tx) error {
		total = readTotal(tx.Bucket(metaBucket))
		return nil
	})
	return total
}

// Clear empties the store and its file.
func (s *thumbStore) Clear() {
	db := s.open()
	if db == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(thumbBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(thumbBucket); err != nil {
			return err
		}
		return writeTotal(tx.Bucket(metaBucket), 0)
	})
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
