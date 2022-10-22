package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

var githubToken = os.Getenv("GITHUB_TOKEN")
var secret = os.Getenv("PINNER_API_TOKEN")
var directory = os.Getenv("IPFS_DIRECTORY")
var addr = os.Getenv("PINNER_ADDR")

func addFileToIPFS(filePath string) (string, error) {
	var outbuf, errbuf strings.Builder
	cmd := exec.Command("ipfs", "add", "--quieter", filePath)

	cmd.Stderr = &errbuf
	cmd.Stdout = &outbuf

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%v: %v", err, errbuf.String())
	}

	out := strings.TrimSuffix(outbuf.String(), "\n")

	return out, nil
}

func getFileName(contentDisposition string) string {
	// Content-Disposition: attachment; filename=bionic-kvm.img.zip; filename*=UTF-8''bionic-kvm.img.zip
	fields := strings.Split(contentDisposition, "; ")
	for _, field := range fields {
		if field[:8] == "filename" {
			return field[9:]
		}
	}

	return ""
}

func downloadFiles(fileURLStr string) (string, error) {
	// Build fileName from fullPath
	client := http.Client{}
	// Create blank file
	tempFile, err := os.CreateTemp("", "pinner-")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Put content on file
	header := map[string][]string{
		"Authorization": {fmt.Sprintf("Bearer %v", githubToken)},
	}
	request, _ := http.NewRequest("GET", fileURLStr, nil)
	request.Header = header
	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%v: HTTP %v", fileURLStr, resp.Status)
	}

	contentDisposition := resp.Header.Get("content-disposition")
	fileName := getFileName(contentDisposition)

	if fileName == "" {
		return "", fmt.Errorf("failed to get filename for %v", fileURLStr)
	}

	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return "", err
	}

	log.Printf("%v (%v) downloaded: %v", fileName, fileURLStr, resp.Status)

	filePath := path.Join(directory, fileName)
	os.Rename(tempFile.Name(), filePath)

	return filePath, nil
}

type fileMeta struct {
	Name string
	Hash string
}

type fileIn struct {
	Name string
	URL  string
}

func processFile(fileName, fileURL string, hashChan chan<- *fileMeta) {
	log.Printf("downloading %v", fileURL)
	filePath, err := downloadFiles(fileURL)
	if err != nil {
		err = errors.Wrap(err, "failed to download file")
		log.Println(err)
		return
	}

	log.Printf("adding file to IPFS %v", filePath)
	fileHash, err := addFileToIPFS(filePath)
	if err != nil {
		err = errors.Wrap(err, "failed to add file to IPFS")
		log.Println(err)
		return
	}
	hashChan <- &fileMeta{
		Name: fileName,
		Hash: fileHash,
	}
}

func processFiles(fileChan <-chan *fileIn) map[string]string {
	hashes := make(map[string]string, 0)
	hashChan := make(chan *fileMeta, 10)

	processingDone := make(chan struct{}, 0)
	go func() {
		for {
			select {
			case file := <-fileChan:
				if file == nil {
					processingDone <- struct{}{}
					return
				}
				processFile(file.Name, file.URL, hashChan)
			}
		}
	}()

	aggregationDone := make(chan struct{}, 0)
	go func() {
		for {
			select {
			case meta := <-hashChan:
				if meta == nil {
					aggregationDone <- struct{}{}
					return
				}
				hashes[meta.Name] = meta.Hash
			}
		}
	}()

	<-processingDone
	close(hashChan)
	<-aggregationDone

	return hashes
}

func processFileList(filesURLs map[string]string) (map[string]string, error) {
	fileChan := make(chan *fileIn, 100)

	go func() {
		for fileName, fileURL := range filesURLs {
			fileChan <- &fileIn{
				Name: fileName,
				URL:  fileURL,
			}
		}
		close(fileChan)
	}()

	hashes := processFiles(fileChan)

	return hashes, nil
}

func authenticate(r *http.Request) error {
	reqSecret := r.Header.Get("Authorization")
	auth := strings.Split(reqSecret, " ")
	if len(auth) != 2 || auth[0] != "Bearer" || auth[1] != secret {
		log.Printf("auth error for %v", r.RemoteAddr)
		return errors.New("authentication failed")
	}

	return nil
}

func wsReader(readChan <-chan []byte, fileChan chan<- *fileIn) {
	for {
		select {
		case message := <-readChan:
			if message == nil {
				log.Println("finished reading msg from websocket")
				return
			}
			files := make(map[string]string, 0)
			err := json.Unmarshal(message, &files)
			if err != nil {
				log.Println("error reading msg from websocket:", err)
				continue
			}
			for fileName, fileURL := range files {
				fileChan <- &fileIn{
					Name: fileName,
					URL:  fileURL,
				}
			}
		}
	}
}

func wsWriter(writeChan chan<- []byte, hashChan <-chan *fileMeta) {
	for {
		select {
		case file := <-hashChan:
			if file == nil {
				log.Println("finished writing to websocket")
				return
			}
			message, err := json.Marshal(file)
			if err != nil {
				log.Println("failed to serialized message:", err)
				continue
			}

			writeChan <- message
		}
	}
}

func wsConnManager(c *websocket.Conn, writeChan chan []byte, readChan chan []byte) {
	go func() {
		for {
			select {
			case msg := <-writeChan:
				if msg == nil {
					return
				}
				log.Println("sending result to client")
				err := c.WriteMessage(websocket.BinaryMessage, msg)
				if err != nil {
					log.Println("failed to write msg to ws conn:", err)
				}
			}
		}
	}()

	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("failed to read message from conn", err)
			close(readChan)
			close(writeChan)
			return
		}
		if mt == websocket.CloseMessage {
			log.Println("connection closed remotely")
			close(readChan)
			close(writeChan)
			return
		}
		if mt != websocket.BinaryMessage {
			log.Println("bad message, not a binary message")
			continue
		}

		readChan <- message
	}
}

func handlerWebsocket(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := authenticate(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Printf("auth error for %v", r.RemoteAddr)
		return
	}
	upgrader := websocket.Upgrader{}

	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	readChan := make(chan []byte, 100)
	writeChan := make(chan []byte, 100)
	hashChan := make(chan *fileMeta, 100)
	fileChan := make(chan *fileIn, 100)

	go wsConnManager(c, writeChan, readChan)
	go wsReader(readChan, fileChan)
	go wsWriter(writeChan, hashChan)

	processingDone := make(chan struct{}, 0)
	go func() {
		for {
			select {
			case file := <-fileChan:
				if file == nil {
					processingDone <- struct{}{}
					return
				}
				go processFile(file.Name, file.URL, hashChan)
			}
		}
	}()

	<-processingDone
}

func handlerSync(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := authenticate(r); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Printf("auth error for %v", r.RemoteAddr)
		return
	}

	bodyBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}
	pr := make(map[string]string)
	err = json.Unmarshal(bodyBytes, &pr)
	if err != nil {
		log.Println(err)
	}

	if len(pr) == 0 {
		log.Println("no file url in request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hashes, err := processFileList(pr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	respBody, _ := json.Marshal(hashes)
	w.Write(respBody)
	return
}

func main() {
	// handle route using handler function
	http.HandleFunc("/sync", handlerSync)
	http.HandleFunc("/websocket", handlerWebsocket)

	// listen to port
	log.Printf("starting server on %v", addr)
	http.ListenAndServe(addr, nil)
}
