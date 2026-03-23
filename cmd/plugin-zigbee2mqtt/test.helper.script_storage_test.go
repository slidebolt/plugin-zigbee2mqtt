//go:build bdd || local

package main

import (
	"encoding/json"
	"testing"

	storage "github.com/slidebolt/sb-storage-sdk"
)

func saveScriptDefinition(t *testing.T, store storage.Storage, name, source string) {
	t.Helper()
	data, err := json.Marshal(map[string]string{
		"type":     "script",
		"language": "lua",
		"name":     name,
		"source":   source,
	})
	if err != nil {
		t.Fatalf("marshal script %s: %v", name, err)
	}
	if err := store.Save(scriptDefBlob{key: "sb-script.scripts." + name, data: data}); err != nil {
		t.Fatalf("save script %s: %v", name, err)
	}
}

type scriptDefBlob struct {
	key  string
	data json.RawMessage
}

func (b scriptDefBlob) Key() string                  { return b.key }
func (b scriptDefBlob) MarshalJSON() ([]byte, error) { return b.data, nil }
