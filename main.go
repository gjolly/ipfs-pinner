package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

const secret = "j9p8QFBusmwFhdxpq97MY6uC3DiQBKZNzrKiGaWa"
const directory = "/home/ubuntu/ubuntu-images"

func addFileToIPFS(filePath string) (string, error) {
	return "", nil
}

func downloadFiles(fileName, fileURLStr string) (string, error) {
	// Build fileName from fullPath
	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}
	// Create blank file
	tempFile, err := os.CreateTemp("", "pinner-")
	if err != nil {
		return "", err
	}

	// Put content on file
	resp, err := client.Get(fileURLStr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	filePath := path.Join(directory, fileName)
	os.Rename(tempFile.Name(), filePath)

	return fileName, nil
}

type fileMeta struct {
	Path string
	Hash string
}

func processFile(filesURLs map[string]string) (map[string]string, error) {
	hashes := make(map[string]string, 0)
	hashChan := make(chan fileMeta, 10)

	for fileName, fileURL := range filesURLs {
		go func(fileName, fileURL string) {
			log.Printf("downloading %v", fileURL)
			filePath, err := downloadFiles(fileName, fileURL)
			if err != nil {
				log.Println(err)
				return hashes, err
			}

			fileHash, err := addFileToIPFS(filePath)
			if err != nil {
				log.Println(err)
				return hashes, err
			}

			hashChan <- fileMeta{filePath, fileHash}
		}()
	}

	hashes[fileName] = fileHash

	return hashes, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	body, err := r.GetBody()
	if err != nil {
		log.Println(err)
	}
	defer body.Close()

	reqSecret := r.Header.Get("Authorization")
	auth := strings.Split(reqSecret, " ")
	if len(auth) != 2 && auth[0] != "Bearer" && auth[1] != secret {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	bodyBytes, err := ioutil.ReadAll(body)
	if err != nil {
		log.Println(err)
	}
	pr := new(PinRequest)
	err = json.Unmarshal(bodyBytes, pr)
	if err != nil {
		log.Println(err)
	}

	if len(pr.Files) == 0 {
		log.Println("no file url in request")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	hashes, err := processFile(pr.Files)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pinResponse := new(PinResponse)
	pinResponse.Hashes = hashes

	respBody, _ := json.Marshal(pinResponse)
	w.Write(respBody)
	w.WriteHeader(http.StatusOK)
	return
}

func main() {
	// handle route using handler function
	http.HandleFunc("/", handler)

	// listen to port
	http.ListenAndServe(":5050", nil)
}
