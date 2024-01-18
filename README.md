#### File upload server that accepts files by chunks
How it works
* Metadata is sent to the server to "register" the file. The server responds with an ID for the file and the desired chunk size
* File is divided into smaller chunks and each chunk is sent to the server one at a time. These chunks are created on the client-side, and each chunk is hashed
* Client sends each chunk to the server along with its chunk number and hash. Server validates the chunk and stores it
* Application signals to the server that the file upload is complete
* Server receives the chunks, validates them, and build them back into the original file. It uses the metadata and chunk information received earlier
* After successfully building file, the server confirms the completion of the upload storing in json as a db some info about uploaded file

-----

**To run server type the following command:**

`go run server.go <host> <port>`

-----
#### To run client type: 

`go run client.go <path to your file> <server host> <port> <maxConcurrentUploads>`

