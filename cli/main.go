package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peterh/liner"
)

var historyFile = filepath.Join(os.TempDir(), ".kvdb_history")

type APIresponse struct {
	Message string `json:"message"`
	Data    string `json:"data"`
	Success bool   `json:"success"`
}

func main() {
	line := liner.NewLiner()
	defer line.Close()

	line.SetCtrlCAborts(true)
	line.SetCompleter(func(line string) (c []string) {
		commands := []string{"SET", "GET", "DELETE", "DEL", "HELP", "EXIT", "PING", "STATS", "SCAN"}
		for _, cmd := range commands {
			if strings.HasPrefix(strings.ToUpper(cmd), strings.ToUpper(line)) {
				c = append(c, cmd)
			}
		}
		return
	})
	if f, err := os.Open(historyFile); err == nil {
		line.ReadHistory(f)
		f.Close()
	}
	fmt.Println("🚀 Welcome to KVDB Interactive CLI v1.0!")
	fmt.Printf("Connected to %s\n", serverURL)
	fmt.Println("Type 'HELP' for commands or 'EXIT' to quit.")
	fmt.Println(strings.Repeat("-", 50))
	for {
		input, err := line.Prompt("kvdb> ")

		if err == liner.ErrPromptAborted {
			fmt.Println("Aborted, Type EXIT to quit.")
			continue
		} else if err != nil {
			fmt.Println("\nGoodbye")
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		line.AppendHistory(input)
		parts := strings.SplitN(input, " ", 3)
		cmd := strings.ToUpper(parts[0])

		switch cmd {
		case "SET":
			if len(parts) < 3 {
				fmt.Println("Usage: SET <key> <value>")
				continue
			}
			doSet(parts[1], parts[2])

		case "GET":
			if len(parts) < 2 {
				fmt.Println("Usage: GET <key>")
				continue
			}
			doGet(parts[1])
		case "DELETE", "DEL":
			if len(parts) < 2 {
				fmt.Println("Usage: DELETE <key>")
				continue
			}
			doDelete(parts[1	])
		case "PING":
			doPing()
		case "HELP":
			printHelp()
		case "STATS":
			doStats()
		case "EXIT", "QUIT", "CLEAR":
			fmt.Println("Goodbye!")
			goto exitLoop

		default:
			fmt.Printf("Unknown command: '%s'. Type HELP for options.\n", cmd)

		}
	}
exitLoop:
	if f, err := os.Create(historyFile); err == nil {
		line.WriteHistory(f)
		f.Close()
	}

}

const serverURL = "http://localhost:8080"

func doSet(key, value string) {
	start := time.Now()
	payload := map[string]string{"key": key, "value": value}
	jsonPayload, _ := json.Marshal(payload)

	resp, err := http.Post(serverURL+"/set", "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Printf("Server Offline %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("OK (%v)\n", time.Since(start))
	} else {
		fmt.Printf("Failed (status: %d)\n", resp.StatusCode)
	}
}

func doGet(key string) {
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("%s/get?key=%s", serverURL, key))
	if err != nil {
		fmt.Printf("Server Offline: %v\n", err)
		return
	}
	defer resp.Body.Close()

	duration := time.Since(start)

	if resp.StatusCode == http.StatusNotFound {
		fmt.Printf("(nil) - Key not found (took %v)\n", duration)
		return
	}

	body, _ := io.ReadAll(resp.Body)
	var apiResp APIresponse
	json.Unmarshal(body, &apiResp)
	fmt.Printf("🔹 \"%s\" (took %v)\n", apiResp.Data, duration)
}

func doDelete(key string) {
	start := time.Now()
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/delete?key=%s", serverURL, key), nil)
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Printf("Server Offline: %v\n", err)
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Deleted %v (took %v)\n", key, time.Since(start))
	} else {
		fmt.Printf("Failed to delete (Status :%d)\n", resp.StatusCode)
	}
}

func doStats() {
	start := time.Now()
	resp, err := http.Get(serverURL + "/stats")
	if err != nil {
		fmt.Printf("Server Offline: %v\n", err)
		return
	}

	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var apiResp APIresponse
	json.Unmarshal(body, &apiResp)

	var stats map[string]any
	json.Unmarshal([]byte(apiResp.Data), &stats)

	fmt.Println("Database Statistics:")
	fmt.Printf("    Partitions: %v\n", stats["partitions"])
	fmt.Printf("    Key in RAM (memtables): %v\n", stats["keys_in_memory"])
	fmt.Printf("    SSTables on Disk: %v\n", stats["sstables_on_disk"])
	fmt.Printf("    (Query took %v)\n", time.Since(start))
}

func printHelp() {
	fmt.Println("--------------------------------------------------------------")
	fmt.Println("  SET <key> <val>  : Save a new key-value pair")
	fmt.Println("  GET <key>        : Retrieve a value by key")
	fmt.Println("  DELETE <key>     : Remove a key")
	fmt.Println("  PING             : Test server connection")
	fmt.Println("  EXIT             : Close the CLI")
	fmt.Println("--------------------------------------------------------------")

}

func doPing() {
	start := time.Now()
	resp, err := http.Get(serverURL + "/ping")
	if err != nil {
		fmt.Printf("Server offline: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Printf("PONG (took %v)\n", time.Since(start))
}
