package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type HTTPServer struct {
	store *Store
	port  int
}

func NewHTTPServer(store *Store, port int) *HTTPServer {
	return &HTTPServer{
		store: store,
		port:  port,
	}
}

type SetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    string `json:"data,omitempty"`
}

func (s *HTTPServer) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST Method Allowed!", http.StatusMethodNotAllowed)
		return
	}

	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JOSN type", http.StatusBadRequest)
		return
	}
	if err := s.store.Set(req.Key, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Success: true, Message: "Key Saved Successfully"})
}

func (s *HTTPServer) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only Get Method Allowed", http.StatusMethodNotAllowed)
		return
	}
	key := r.URL.Query().Get("key")

	val, found := s.store.Get(key)
	w.Header().Set("Content-Type", "application/json")

	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(Response{Success: false, Message: "Key not found"})
		return
	}
	json.NewEncoder(w).Encode(Response{Success: true, Data: val})
}

func (s *HTTPServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Only Delete Method Allowed!", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "Missing 'key' Parameter", http.StatusBadRequest)
		return
	}

	if err := s.store.Delete(key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Success: true, Message: "Key Deleted Successfully."})
}

// handlePing processes GET /ping
func (s *HTTPServer) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Success: true, Message: "PONG"})
}

// handleStats processes GET /stats
func (s *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET Method Allowed", http.StatusMethodNotAllowed)
		return
	}

	totalMemKeys := 0
	totalSSTables := 0

	// Gather stats from all partitions
	for _, p := range s.store.partitions {
		p.mu.RLock()
		totalMemKeys += p.memtable.Size
		totalSSTables += len(p.sstables)
		p.mu.RUnlock()
	}

	statsData := map[string]any{
		"partitions":       s.store.numShards,
		"keys_in_memory":   totalMemKeys,
		"sstables_on_disk": totalSSTables,
	}

	// Convert the map to a JSON string so it fits in our Response struct
	statsBytes, _ := json.Marshal(statsData)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{Success: true, Data: string(statsBytes)})
}

func (s *HTTPServer) Start() error {
	http.HandleFunc("/set", s.handleSet)
	http.HandleFunc("/get", s.handleGet)
	http.HandleFunc("/delete", s.handleDelete)

	http.HandleFunc("/ping", s.handlePing)
	http.HandleFunc("/stats", s.handleStats)

	addr := fmt.Sprintf(":%d", s.port)
	fmt.Printf("KVDB Server running on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, nil)
}
