package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/gorilla/websocket"
)

var secret = os.Getenv("PINNER_API_TOKEN")

var apiAddress = flag.String("addr", "ws://localhost:5050/websocket", "http service address")
var filesInfoPath = flag.String("file", "", "JSON file with upload info")

func pinFiles(addr string, filesMetadata map[string]string) error {
	header := map[string][]string{
		"Authorization": {fmt.Sprintf("Bearer %v", secret)},
	}

	c, _, err := websocket.DefaultDialer.Dial(addr, header)
	if err != nil {
		return err
	}
	defer c.Close()

	for fileName, fileURL := range filesMetadata {
		// this is just to create one request per file
		// to make sure the server process them in parallel
		// actually...I am not sure this is needed
		message, _ := json.Marshal(map[string]string{
			fileName: fileURL,
		})

		c.WriteMessage(websocket.BinaryMessage, message)
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(filesMetadata); i++ {
		mt, message, err := c.ReadMessage()
		if err != nil {
			return err
		}
		if mt != websocket.BinaryMessage {
			return errors.New("unknown message")
		}
		fmt.Printf("%s\n", message)
	}

	c.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(
			websocket.CloseNormalClosure,
			"",
		),
	)
	return nil
}

func main() {
	flag.Parse()
	if *filesInfoPath == "" {
		log.Fatal("no file provided")
	}

	f, err := os.Open(*filesInfoPath)
	if err != nil {
		log.Fatal("filed to open metadata file:", err)
	}
	fileContent, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal("filed to read metadata file:", err)
	}

	filesMetadata := make(map[string]string, 0)
	err = json.Unmarshal(fileContent, &filesMetadata)
	if err != nil {
		log.Fatal("failed to parse file:", err)
	}

	err = pinFiles(*apiAddress, filesMetadata)
	if err != nil {
		log.Fatal("failed to pin:", err)
	}
}
