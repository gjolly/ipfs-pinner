package main

import (
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

func pinFiles(addr string, filesMetadata []byte) error {
	header := map[string][]string{
		"Authorization": {fmt.Sprintf("Bearer %v", secret)},
	}

	c, _, err := websocket.DefaultDialer.Dial(addr, header)
	if err != nil {
		return err
	}
	defer c.Close()

	c.WriteMessage(websocket.BinaryMessage, filesMetadata)
	if err != nil {
		return err
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

	err = pinFiles(*apiAddress, fileContent)
	if err != nil {
		log.Fatal("failed to pin:", err)
	}
}
