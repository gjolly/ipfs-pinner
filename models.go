package main

type PinRequest struct {
	Files map[string]string `json:"files"`
}

type PinResponse struct {
	Hashes map[string]string `json:"hashes"`
}
