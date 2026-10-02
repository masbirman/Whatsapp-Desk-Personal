package main

import (
	"encoding/json"
	"errors"
)

// M6-02 manual story save. There is deliberately no status discovery,
// listing, or archiving: the only path is one explicit user action that
// forwards the currently open viewer's media through the same validated
// saver as normal downloads (path traversal, basename, and size guards).
// If the viewer identity is missing the page refuses to send anything.

type storySaveEnvelope struct {
	OK      bool   `json:"ok"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

func storySaveJSON(raw string) string {
	failure := func(msg string) string {
		data, _ := json.Marshal(storySaveEnvelope{OK: false, Error: msg})
		return string(data)
	}
	var request struct {
		DataURI  string `json:"data_uri"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return failure("invalid story save request")
	}
	if request.Filename == "" {
		request.Filename = "status-" + nowStampFileSafe() + ".bin"
	}
		if request.Filename == "" || len(request.Filename) > maxBridgeFilenameBytes {
		return failure("invalid story filename")
	}
	path, err := saveDownloadedFileFromBridge(request.Filename, request.DataURI)
	if err != nil {
		envelope := storySaveEnvelope{OK: false, Error: err.Error()}
		data, _ := json.Marshal(envelope)
		return string(data)
	}
	data, marshalErr := json.Marshal(storySaveEnvelope{OK: true, Path: path})
	if marshalErr != nil {
		data, _ := json.Marshal(storySaveEnvelope{OK: false, Error: "result could not be encoded"})
		return string(data)
	}
	return string(data)
}

// Explicitly rejected: anything resembling automated story retrieval. This
// function exists to document the boundary, not to be called.
var errStoryAutomationUnsupported = errors.New("automatic story access is not supported")
