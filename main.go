package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

const (
	TOMBSTONE     = "---deleted---"
	MEMTABLE_SIZE = 3
	WAL_FILE      = "wal.log"
)

type Store struct {
	wal      *os.File
	memtable *Skiplist
	mu       sync.RWMutex
	indexes  map[string][]IndexEntry
	sstables []string
	blooms   map[string]*BloomFilters
}

type IndexEntry struct {
	Key    string
	Offset int64
}

type LogEntry struct {
	Cmd   string
	Key   string
	Value string
}

func NewStore() (*Store, error) {
	file, err := os.OpenFile(WAL_FILE, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0664)
	if err != nil {
		return nil, err
	}

	s := &Store{
		wal:      file,
		memtable: NewSkiplist(),
		mu:       sync.RWMutex{},
		indexes:  make(map[string][]IndexEntry),
		sstables: []string{},
		blooms:   make(map[string]*BloomFilters),
	}

	err = s.Replay()
	if err != nil {
		fmt.Println("error replaying log")
	}
	err = s.LoadSSTables()
	if err != nil {
		fmt.Println("error loading sstables")
	}
	return s, nil
}

func (s *Store) Replay() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.wal.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(s.wal)
	for scanner.Scan() {
		var entry LogEntry
		err := json.Unmarshal(scanner.Bytes(), &entry)
		if err != nil {
			return err
		}
		switch entry.Cmd {
		case "SET":
			s.memtable.Insert(entry.Key, entry.Value)
		case "DELETE":
			s.memtable.Insert(entry.Key, TOMBSTONE)
		}
	}
	return scanner.Err()
}

func (s *Store) LoadSSTables() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(".")
	if err != nil {
		return err
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sst-") && strings.HasSuffix(e.Name(), ".db") {
			filename := e.Name()
			s.sstables = append(s.sstables, filename)

			file, err := os.Open(filename)
			if err != nil {
				continue
			}
			var index []IndexEntry
			var currentOffset int64

			reader := bufio.NewReader(file)

			for {
				lineBytes, err := reader.ReadBytes('\n')
				if err != nil {
					break
				}

				parts := strings.SplitN(string(lineBytes), ",", 2)
				if len(parts) >= 2 {
					index = append(index, IndexEntry{
						Key:    parts[0],
						Offset: currentOffset,
					})
				}

				currentOffset += int64(len(lineBytes))
			}
			file.Close()
			s.indexes[filename] = index

			bf := NewBloomFilter(len(index), 0.01)
			for _, entry := range index {
				bf.Add(entry.Key)
			}
			s.blooms[filename] = bf
		}
	}
	sort.Strings(s.sstables)
	return nil
}

func (s *Store) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := LogEntry{
		Cmd:   "SET",
		Key:   key,
		Value: value,
	}
	bytes, _ := json.Marshal(entry)

	s.wal.Write(bytes)
	s.wal.WriteString("\n")
	s.wal.Sync()

	s.memtable.Insert(key, value)

	if s.memtable.Size >= MEMTABLE_SIZE {
		return s.Flush()
	}
	return nil
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.memtable.Search(key); ok {

		if val == TOMBSTONE {
			return "", false
		}

		return val, true
	}
	for i := len(s.sstables) - 1; i >= 0; i-- {
		val, ok := s.SearchSSTables(key, s.sstables[i])
		if ok {
			if val == TOMBSTONE {
				return "", false
			}
			return val, ok
		}
	}
	return "", false
}

func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := LogEntry{
		Cmd: "DELETE",
		Key: key,
	}

	bytes, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	s.wal.Write(bytes)
	s.wal.WriteString("\n")

	if err := s.wal.Sync(); err != nil {
		return err
	}

	s.memtable.Insert(key, TOMBSTONE)

	if s.memtable.Size >= MEMTABLE_SIZE {
		return s.Flush()
	}

	return nil
}

func (s *Store) Flush() error {

	bf := NewBloomFilter(s.memtable.Size, 0.01)

	filename := fmt.Sprintf("sst-%d.db", len(s.sstables))
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	iter := s.memtable.NewIterator()

	var currentOffset int64
	var index []IndexEntry

	for iter.Next() {
		index = append(index, IndexEntry{
			Key:    iter.Key(),
			Offset: currentOffset,
		})

		bf.Add(iter.Key())

		line := fmt.Sprintf("%s,%s\n", iter.Key(), iter.Value())

		n, err := file.WriteString(line)
		if err != nil {
			return err
		}

		currentOffset += int64(n)
	}
	s.indexes[filename] = index
	s.blooms[filename] = bf
	s.sstables = append(s.sstables, filename)

	fmt.Printf("Flushed %s with index size %d\n", filename, len(index))

	s.memtable = NewSkiplist()
	if err := s.wal.Truncate(0); err != nil {
		return nil
	}

	if _, err = s.wal.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	return err
}

func (s *Store) SearchSSTables(key, filename string) (string, bool) {

	if bf, ok := s.blooms[filename]; ok {
		if !bf.MightContain(key) {
			return "", false
		}
	}

	index, ok := s.indexes[filename]
	if !ok {
		return "", false
	}

	startOffset := int64(0)

	for _, entry := range index {
		if entry.Key > key {
			break
		}
		startOffset = entry.Offset
	}

	file, err := os.Open(filename)
	if err != nil {
		return "", false
	}

	defer file.Close()

	_, err = file.Seek(startOffset, io.SeekStart)
	if err != nil {
		return "", false
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ",", 2)
		if len(parts) == 2 {
			if parts[0] == key {
				return parts[1], true
			}

			if parts[0] > key {
				return "", false
			}
		}
	}
	return "", false
}

func main() {
	// Nuke old state for a clean run
	os.Remove("wal.log")
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".db") || strings.HasSuffix(e.Name(), ".tmp") {
			os.Remove(e.Name())
		}
	}

	s, err := NewStore()
	if err != nil {
		panic(err)
	}

	fmt.Println("--- Writing Batch 1 (Forces Flush 1) ---")
	s.Set("user:1", "Alice")
	s.Set("user:2", "Bob")
	s.Set("user:3", "Charlie") // Memtable hits 3 -> Flushes to sst-0.db

	fmt.Println("\n--- Writing Batch 2 (Forces Flush 2) ---")
	s.Set("user:4", "Dave")
	s.Set("user:2", "Bob_V2") // Overwrite older value
	s.Delete("user:1")        // Tombstone older value
	s.Set("user:5", "Eve")    // Memtable hits 3 -> Flushes to sst-1.db

	fmt.Printf("\nBefore Compaction, SSTables count: %d\n", len(s.sstables))

	fmt.Println("\n--- Triggering Compaction ---")
	if err := s.Compaction(); err != nil {
		panic(err)
	}

	fmt.Printf("\nAfter Compaction, SSTables count: %d\n", len(s.sstables))

	fmt.Println("\n--- Verifying Data ---")

	// user:1 should be gone completely
	if val, ok := s.Get("user:1"); ok {
		fmt.Printf("FAIL: user:1 should be deleted, got '%s'\n", val)
	} else {
		fmt.Println("OK: user:1 is successfully deleted")
	}

	// user:2 should be the newer version
	if val, ok := s.Get("user:2"); ok {
		fmt.Printf("OK: user:2 is '%s'\n", val)
	} else {
		fmt.Println("FAIL: user:2 not found")
	}

	// user:3 should be untouched from the first batch
	if val, ok := s.Get("user:3"); ok {
		fmt.Printf("OK: user:3 is '%s'\n", val)
	} else {
		fmt.Println("FAIL: user:3 not found")
	}
}
