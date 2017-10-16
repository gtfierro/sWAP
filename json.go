package main

import (
	"encoding/json"
)

type SmapMessage struct {
	UUID       string                 `json:"uuid"`
	Path       string                 `json:"Path"`
	Properties map[string]interface{} `json:"Properties"`
	Metadata   map[string]interface{} `json:"Metadata"`
	Readings   [][]json.Number        `json:"Readings"`
}
