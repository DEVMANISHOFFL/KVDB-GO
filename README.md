
# KVDB.GO - A Distributed, LSM-Based Key-Value Store

KVDB.GO is a high-throughput, distributed key-value storage engine written entirely in Go.
Designed around the principles of Log-Structured Merge (LSM) Trees, it provides ultra-fast
sequential write performance, strict crash durability via a Write-Ahead Log (WAL), and
horizontal scalability using Consistent Hashing.

This project was engineered to explore the deep internals of modern storage engines like
Cassandra, RocksDB, and DynamoDB, focusing heavily on architectural trade-offs such as write
amplification, lock contention, OS-level file buffering, and Go garbage collection pressure.


## Architectural Overview

            ┌──────────────┐
            │    Client    │
            │ (HTTP / CLI) │
            └──────┬───────┘
                   │
                   ▼
        ┌──────────────────────┐
        │  Consistent Hashing  │
        │   (Partition Router) │
        └─────────┬────────────┘
                  ▼
      ┌────────────────────────────┐
      │        Storage Node        │
      │                            │
      │  ┌──────────────┐          │
      │  │     WAL      │ ◄── Write│
      │  └──────┬───────┘          │
      │         ▼                  │
      │  ┌──────────────┐          │
      │  │  Memtable    │          │
      │  │ (Skiplist)   │          │
      │  └──────┬───────┘          │
      │         ▼ Flush            │
      │  ┌──────────────┐          │
      │  │  SSTables    │ ◄── Read │
      │  └──────┬───────┘          │
      │         ▼                  │
      │  ┌──────────────┐          │
      │  │ Compaction   │          │
      │  └──────────────┘          │
      └────────────────────────────┘

## Core Components Breakdown

### 1. Write-Ahead Log (WAL)
To guarantee ACID durability without sacrificing write speed, every mutation is first appended
to the WAL. During startup, the WAL is replayed to
reconstruct the exact in-memory state prior to the crash.

### 2. Memtable (Concurrent Skiplist)
In-memory writes are stored in a Skiplist rather than a Red-Black or B-Tree. Skiplists offer
search and insertion while requiring significantly less complex locking mechanics. In
a highly concurrent Go environment, modifying a B-Tree requires locking the root and
subsequent nodes during rebalancing, whereas a Skiplist allows highly localized, fine-grainedlocking, drastically increasing parallel write throughput.

### 3. SSTables & Sparse Indexing
When the Memtable reaches a pre-configured memory threshold, it is flushed to disk as an
immutable Sorted String Table (SSTable). SSTables are divided into 4KB data blocks. Instead of
loading the entire file into memory to find a key, KVDB.GO uses a Sparse Index—mapping the
first key of every block to its disk offset—allowing the system to locate data with a single binary
search and exactly one disk seek.

### 4. Bloom Filters
Because LSM trees spread data across multiple files, a read request might require checking
several SSTables on disk. To prevent catastrophic read amplification, each SSTable maintains a
probabilistic Bloom Filter in memory. If the filter returns false, the disk seek is entirely bypassed,
saving immense I/O resources.

### 5. Background Compaction
Deletes in an LSM tree are simply writes with a nil value (known as tombstones). Over time,
tombstones and updated values cause space amplification and degrade read performance. A
background goroutine continuously performs Leveled Compaction, merging overlapping
SSTables, purging obsolete keys, and writing fresh, consolidated tables to disk without locking
the main execution thread.

### 6. Consistent Hashing Cluster
Horizontal scaling is achieved via a ring-based consistent hashing algorithm using FNV-1a. To
prevent the uneven data distribution common in basic ring topologies, the system implements
Virtual Nodes (vnodes). This ensures that when a node crashes or joins, the resharding load is
distributed evenly across all surviving nodes rather than overwhelming a single neighbor.

### 7. HTTP API & Interactive CLI
The database is accessible via a robust REST API, backed by Go's native net/http multiplexer
with connection pooling. Furthermore, the repository includes an interactive CLI client
featuring command history, syntax highlighting, and auto-complete for rapid cluster
administration.

## 1. Installation and Usage 

### Prerequisites
```bash
 Go 1.20+
```

### Download dependencies
```bash
go mod download 
```

### Run the server (starts on port 8080)
```bash
go run .
```

## 2. API Usage (cURL)
### Commands

### SET
```bash
curl -X POST http://localhost:8080/set \
     -H "Content-Type: application/json" \
     -d '{"key": "user_1", "value": "Manish"}'
```
### Output
```JSON
{"success":true,"message":"Key Saved Successfully"}
```

### GET
```bash
curl "http://localhost:8080/get?key=user_1"
```

### Output
```JSON
{"success":true,"data":"Manish"}
```

### Delete
```bash
curl -X DELETE "http://localhost:8080/delete?key=user_1"
```
### Output
```bash
{"success":true,"message":"Key Deleted Successfully."}
```

### Stats
```bash
curl http://localhost:8080/stats
```

### Output
```JSON
{"success":true,"data":"{\
"keys_in_memory\":0,\
"partitions\":4,\
"sstables_on_disk\":0
}"}
```

## 3. CLI Usage

```bash
go run cli/main.go cli/cleaner.go
```
### CLI COMMANDS
```
  SET <key> <val>  : Save a new key-value pair
  GET <key>        : Retrieve a value by key
  DELETE <key>     : Remove a key
  PING             : Test server connection
  EXIT             : Close the CLI
  ```

##  Deep Dive: What I Learned (Architectural Tradeoffs)
### Engineering Manifesto: Systems Design and Architectural Tradeoffs
Building a database from scratch removes modern framework layers. It makes an engineer face the tough realities of file systems, memory management, and concurrent design. This system was created by carefully balancing competing factors: memory use versus disk I/O, sequential appending versus random access, and read versus write amplification.

### Here are the main decisions and trade-offs made during development:
- **Storage Engine (LSM Tree vs. B-Tree):** Traditional B-Trees are optimized for reading but face significant Write Amplification and page fragmentation when handling heavy write loads because of the "read-modify-write" cycle. KVDB uses a Log-Structured Merge (LSM) Tree, batching writes in memory and flushing them as immutable SSTables. This change turns slow random disk I/O into fast sequential appends.
- **In-Memory Concurrency (Skiplist vs. B-Tree):** For the in-memory Memtable, we chose a probabilistic Skiplist instead of a B-Tree. Modifying a B-Tree concurrently often requires locking large parts of the tree during node splits. A Skiplist allows for more localized and precise locking, which significantly boosts parallel write rates. Additionally, we used careful pointer management and value semantics to take advantage of Go's escape analysis, keeping short-lived objects on the stack to avoid "stop-the-world" Garbage Collection pauses. 
- **Durability & Torn Writes (WAL):** To withstand power losses without damaging data, all writes are added sequentially to a Write-Ahead Log (WAL) before they are confirmed. To guard against "torn writes" (where a physical disk sector is only partially written during a crash), the WAL ensures strict serialization.
- **Solving Read Amplification (Bloom Filters):** The main downside of an LSM tree is that reading a key might require scanning multiple files. To avoid costly disk seeks for non-existent keys, KVDB creates a Bloom Filter for each SSTable. This mathematically guarantees that if a key is absent, the system can skip unnecessary I/O. If a key is present, a sparse index maps it directly to its disk offset. 
- **The Compaction Tax:** Because LSM trees are append-only, deletes are stored as tombstones. To avoid running out of disk space and to maintain read speeds, a background goroutine performs Leveled Compaction. It continuously merges overlapping SSTables and removes tombstones, carefully balancing the process to prevent stalling the main ingestion thread.
- **Data Partitioning (Sharding):** To scale horizontally and avoid global lock contention, data is divided into distinct partitions using an FNV-based hashing algorithm. This ensures an even distribution of the dataset and allows parallel operations across independent shards.

### Summary of Engineering Principles

Building this engine reinforced a fundamental truth of system design: **complexity cannot be destroyed, it can only be moved.**

- If you desire ultra-fast writes, you must move the complexity to the read path (LSM Trees).
- If you desire fast reads on an LSM tree, you must move the complexity to background CPU processing (Compaction & Bloom Filters).
- If you desire high concurrency, you must accept the complexity of data partitioning and fine-grained locking.
