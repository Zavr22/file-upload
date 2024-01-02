package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type FileMetadata struct {
	ID          string `json:"id"`
	FileName    string `json:"fileName"`
	FileSize    int64  `json:"fileSize"`
	FileHash    string `json:"fileHash"`
	ChunkSize   int    `json:"chunkSize"`
	TotalChunks int    `json:"totalChunks"`
}

var (
	filesMetadata = make(map[string]FileMetadata)
	metadataMutex = &sync.Mutex{}
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: server <ip> <port>")
		os.Exit(1)
	}

	ip, port := os.Args[1], os.Args[2]
	http.HandleFunc("/register_file", registerFileHandler)
	http.HandleFunc("/upload_chunk/", uploadChunkHandler)
	http.HandleFunc("/complete_upload/", completeUploadHandler)

	fmt.Printf("Starting server on %s:%s\n", ip, port)
	if err := http.ListenAndServe(ip+":"+port, nil); err != nil {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}

func registerFileHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received complete upload request for:", r.URL.Path)
	if r.Method != "POST" {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}

	var metadata FileMetadata
	err := json.NewDecoder(r.Body).Decode(&metadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	metadata.ID = generateUniqueID()
	metadata.ChunkSize = calculateChunkSize(metadata.FileSize)
	metadata.TotalChunks = int(math.Ceil(float64(metadata.FileSize) / float64(metadata.ChunkSize)))

	metadataMutex.Lock()
	filesMetadata[metadata.ID] = metadata
	metadataMutex.Unlock()

	response, err := json.Marshal(metadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

func uploadChunkHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received complete upload request for:", r.URL.Path)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
	} else {
		fmt.Println("Current working directory:", cwd)
	}
	if r.Method != "POST" {
		http.Error(w, "Only POST method is allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	fileID, chunkNumber := parts[2], parts[3]

	chunkHash := r.Header.Get("Chunk-Hash")
	if chunkHash == "" {
		http.Error(w, "Chunk hash is missing", http.StatusBadRequest)
		return
	}
	num, err := strconv.Atoi(chunkNumber)
	if err != nil {
		http.Error(w, "Chunk hash number is missing", http.StatusBadRequest)
		return
	}
	chunkFileName := fmt.Sprintf("%s_part_%d", fileID, num)
	fmt.Printf("Saving chunk file: %s\n", chunkFileName)

	chunkFile, err := os.Create(chunkFileName)
	if err != nil {
		fmt.Printf("Error creating chunk file: %v\n", err)
		http.Error(w, "Error creating file", http.StatusInternalServerError)
		return
	}
	defer chunkFile.Close()

	hasher := sha256.New()
	tee := io.TeeReader(r.Body, hasher)
	if _, err := io.Copy(chunkFile, tee); err != nil {
		http.Error(w, "Error writing to file", http.StatusInternalServerError)
		return
	}

	if fmt.Sprintf("%x", hasher.Sum(nil)) != chunkHash {
		http.Error(w, "Chunk hash mismatch", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func completeUploadHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Received complete upload request for:", r.URL.Path)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
	} else {
		fmt.Println("Current working directory:", cwd)
	}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	fileID := parts[2]

	metadataMutex.Lock()
	metadata, ok := filesMetadata[fileID]
	metadataMutex.Unlock()

	if !ok {
		fmt.Println("File metadata not found for ID:", fileID)
		http.Error(w, "File metadata not found", http.StatusBadRequest)
		return
	}

	finalFileName := fmt.Sprintf("final_%s", metadata.FileName)
	finalFile, err := os.Create(finalFileName)
	if err != nil {
		fmt.Println("Error creating final file:", err)
		http.Error(w, "Error creating final file", http.StatusInternalServerError)
		return
	}
	defer finalFile.Close()
	fmt.Println(metadata.TotalChunks)
	if fileID != metadata.ID {
		http.Error(w, "id are not the same", http.StatusInternalServerError)
		return
	}

	for i := 1; i <= metadata.TotalChunks; i++ {
		chunkFileName := fmt.Sprintf("%s_part_%d", fileID, i)
		fmt.Printf("Attempting to open chunk file: %s\n", chunkFileName)

		if _, err := os.Stat(chunkFileName); os.IsNotExist(err) {
			fmt.Printf("Chunk file does not exist: %s\n", chunkFileName)
			http.Error(w, "Chunk file does not exist", http.StatusInternalServerError)
			return
		}

		chunkFile, err := os.Open(chunkFileName)
		if err != nil {
			fmt.Printf("Error opening chunk file %d: %v\n", i, err)
			http.Error(w, fmt.Sprintf("Error opening chunk file %d: %v", i, err), http.StatusInternalServerError)
			return
		}

		if _, err := io.Copy(finalFile, chunkFile); err != nil {
			chunkFile.Close()
			fmt.Println("Error writing to final file:", err)
			http.Error(w, "Error writing to final file", http.StatusInternalServerError)
			return
		}

		chunkFile.Close()
		os.Remove(chunkFileName)
	}

	if err := finalFile.Sync(); err != nil {
		fmt.Println("Error during final file sync:", err)
		http.Error(w, "Error finalizing file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	finalFile.Seek(0, 0)
	finalHash, err := calculateFileHash(finalFile)
	if err != nil {
		fmt.Println("Error calculating final file hash:", err)
		http.Error(w, "Error calculating final file hash: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if fmt.Sprintf("%x", finalHash) != metadata.FileHash {
		fmt.Println("Final file hash mismatch")
		http.Error(w, "Final file hash mismatch", http.StatusBadRequest)
		return
	}

	if err := updateFileInfoDB(metadata); err != nil {
		fmt.Println("Error updating fileInfoDB:", err)
		http.Error(w, "Error updating fileInfoDB: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func generateUniqueID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}

func calculateChunkSize(fileSize int64) int {
	rand.Seed(time.Now().UnixNano())
	const minChunkSize = 100 * 1024
	const maxChunkSize = 4 * 1024 * 1024
	randomChunkSize := rand.Intn(maxChunkSize-minChunkSize+1) + minChunkSize
	if int64(randomChunkSize) > fileSize {
		return int(fileSize)
	}

	return randomChunkSize
}

func calculateFileHash(file *os.File) ([]byte, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

func updateFileInfoDB(metadata FileMetadata) error {
	fileInfoDB := "fileInfoDB.json"
	fileInfoMutex := &sync.Mutex{}

	fileInfoMutex.Lock()
	defer fileInfoMutex.Unlock()

	var fileInfos []FileMetadata

	data, err := ioutil.ReadFile(fileInfoDB)
	if err != nil {
		json.Unmarshal(data, &fileInfos)
		return err
	}

	fileInfos = append(fileInfos, metadata)

	newData, err := json.Marshal(fileInfos)
	if err != nil {
		fmt.Println("Error marshaling file info:", err)
		return err
	}

	if err := ioutil.WriteFile(fileInfoDB, newData, 0644); err != nil {
		fmt.Println("Error writing to file info DB:", err)
		return err
	}
	return nil
}
