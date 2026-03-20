package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

func (s *Store) Compaction() error {
	s.mu.RLock()

	if len(s.sstables) < 2 {
		s.mu.RUnlock()
		return nil
	}

	fileA := s.sstables[0]
	fileB := s.sstables[1]
	s.mu.RUnlock()

	fA, err := os.Open(fileA)
	if err != nil {
		return err
	}
	fB, err := os.Open(fileB)
	if err != nil {
		return err
	}
	defer fA.Close()
	defer fB.Close()

	scA := bufio.NewScanner(fA)
	scB := bufio.NewScanner(fB)

	hasA := scA.Scan()
	hasB := scB.Scan()

	tempName := "compact.tmp"
	fOut, _ := os.Create(tempName)

	var index []IndexEntry
	var currentOffset int64 = 0

	writeLine := func(line string) {
		parts := strings.SplitN(line, ",", 2)
		if len(parts) == 2 {

			key, val := parts[0], parts[1]

			if val == TOMBSTONE {
				return
			}

			index = append(index, IndexEntry{Key: key, Offset: currentOffset})

			n, _ := fOut.WriteString(line + "\n")
			currentOffset += int64(n)
		}
	}

	for hasA && hasB {
		lineA := scA.Text()
		lineB := scB.Text()

		keyA := strings.SplitN(lineA, ",", 2)[0]
		keyB := strings.SplitN(lineB, ",", 2)[0]

		if keyA > keyB {
			writeLine(lineB)
			hasB = scB.Scan()
		} else if keyA < keyB {
			writeLine(lineA)
			hasA = scA.Scan()
		} else {
			writeLine(lineB)
			hasA = scA.Scan()
			hasB = scB.Scan()
		}
	}

	for hasA {
		writeLine(scA.Text())
		hasA = scA.Scan()
	}

	for hasB {
		writeLine(scB.Text())
		hasB = scB.Scan()
	}

	fOut.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	newName := fmt.Sprintf("sst-compacted-%d.db", time.Now().UnixNano())
	os.Rename(tempName, newName)

	s.sstables = append([]string{newName}, s.sstables[2:]...)

	s.indexes[newName] = index
	delete(s.indexes, fileA)
	delete(s.indexes, fileB)

	os.Remove(fileA)
	os.Remove(fileB)

	fmt.Printf("[Compaction] Finished. Created %s. Dropped %s and %s\n", newName, fileA, fileB)
	return nil
}
