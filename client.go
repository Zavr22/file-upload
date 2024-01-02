package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

type FileInfo struct {
	FileName string `json:"fileName"`
	FileSize int64  `json:"fileSize"`
	FileHash string `json:"fileHash"`
}

type RegistrationResponse struct {
	ID          string `json:"id"`
	ChunkSize   int    `json:"chunkSize"`
	TotalChunks int    `json:"totalChunks"`
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: send_file <file_path> <server_ip> <server_port>")
		os.Exit(1)
	}

	filePath, serverIP, serverPort := os.Args[1], os.Args[2], os.Args[3]

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Printf("Error getting file info: %v\n", err)
		os.Exit(1)
	}
	if fileInfo.Size() == 0 {
		fmt.Println("File is empty")
		os.Exit(1)
	}

	fileHash, err := calculateHash(file)
	if err != nil {
		fmt.Printf("Error calculating file hash: %v\n", err)
		os.Exit(1)
	}

	fileMetadata := FileInfo{
		FileName: filepath.Base(filePath),
		FileSize: fileInfo.Size(),
		FileHash: fmt.Sprintf("%x", fileHash),
	}

	regResponse, err := registerFile(serverIP, serverPort, fileMetadata)
	if err != nil {
		fmt.Printf("Error registering file: %v\n", err)
		os.Exit(1)
	}

	err = sendFileChunks(file, serverIP, serverPort, regResponse.ID, regResponse.ChunkSize)
	if err != nil {
		fmt.Printf("Error sending file chunks: %v\n", err)
		os.Exit(1)
	}

	completeUpload(serverIP, serverPort, regResponse.ID)
}

func calculateHash(file *os.File) ([]byte, error) {
	hasher := sha256.New()
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(hasher, file); err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

func registerFile(serverIP, serverPort string, metadata FileInfo) (*RegistrationResponse, error) {
	url := fmt.Sprintf("http://%s:%s/register_file", serverIP, serverPort)
	jsonData, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var regResponse RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResponse); err != nil {
		return nil, err
	}

	return &regResponse, nil
}

func sendFileChunks(file *os.File, serverIP, serverPort, fileID string, chunkSize int) error {
	buffer := make([]byte, chunkSize)
	fmt.Println(chunkSize)
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Printf("Error seeking to the beginning of the file: %v\n", err)
		return err
	}

	for chunkNumber := 1; ; chunkNumber++ {
		bytesRead, err := file.Read(buffer)
		if bytesRead == 0 {
			fmt.Println("No more data to read, exiting loop")
			break
		}
		if err != nil {
			if err == io.EOF {
				fmt.Println("Reached end of file")
				break
			}
			fmt.Printf("Error reading file: %v\n", err)
			return err
		}

		chunkData := buffer[:bytesRead]
		chunkHash := sha256.Sum256(chunkData)
		fmt.Printf("Sending chunk %d (hash: %x)\n", chunkNumber, chunkHash)

		err = sendChunk(serverIP, serverPort, fileID, chunkNumber, chunkData, fmt.Sprintf("%x", chunkHash))
		if err != nil {
			fmt.Printf("Error sending chunk %d: %v\n", chunkNumber, err)
			return err
		}
	}
	return nil
}

func sendChunk(serverIP, serverPort, fileID string, chunkNumber int, chunkData []byte, chunkHash string) error {
	url := fmt.Sprintf("http://%s:%s/upload_chunk/%s/%d", serverIP, serverPort, fileID, chunkNumber)
	fmt.Printf("Preparing to send request to URL: %s\n", url)

	request, err := http.NewRequest("POST", url, bytes.NewReader(chunkData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return err
	}

	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Chunk-Hash", chunkHash)

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("Request sent, response status: %s\n", resp.Status)
	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("Server returned non-OK status: %d, response: %s\n", resp.StatusCode, string(body))
		return fmt.Errorf("server returned non-OK status: %d", resp.StatusCode)
	}
	return nil
}

func completeUpload(serverIP, serverPort, fileID string) {
	url := fmt.Sprintf("http://%s:%s/complete_upload/%s", serverIP, serverPort, fileID)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error completing upload: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error during upload completion: server returned non-OK status: %d\n", resp.StatusCode)
	}
	fmt.Println("File upload completed successfully")
}
