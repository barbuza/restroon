package roonlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

const (
	mooRequest  = "REQUEST"
	mooContinue = "CONTINUE"
	mooComplete = "COMPLETE"

	mooSubscribed = "Subscribed"
	mooChanged    = "Changed"
	mooSuccess    = "Success"
)

func splitMOOResponse(data []byte) ([]byte, []byte) {
	chunks := bytes.SplitN(data, []byte("\n\n"), 2)
	return chunks[0], chunks[1]
}

func parseMOOHead(headers []byte) (string, string) {
	chunks := bytes.SplitN(headers, []byte("\n"), 2)
	head := chunks[0]
	headChunks := bytes.SplitN(head, []byte(" "), 3)
	return string(headChunks[1]), string(headChunks[2])
}

func getRequestID(headers []byte) (int, error) {
	reg := regexp.MustCompile(`(?i)Request-Id: (\d+)`)
	matches := reg.FindSubmatch(headers)
	if len(matches) != 2 {
		return -1, fmt.Errorf("cant find Request-Id header")
	}
	requestID, err := strconv.ParseInt(string(matches[1]), 10, 64)
	if err != nil {
		return -1, err
	}
	return int(requestID), nil
}

func parseMOOResponse(data []byte) (int, string, string, []byte, error) {
	headers, body := splitMOOResponse(data)
	head1, head2 := parseMOOHead(headers)
	requestID, err := getRequestID(headers)
	if err != nil {
		return -1, "", "", nil, err
	}
	if err != nil {
		return -1, "", "", nil, err
	}
	return requestID, head1, head2, body, nil
}

func createMooMessage(w io.Writer, kind string, requestID int, command string, body interface{}) {
	w.Write([]byte(fmt.Sprintf("MOO/1 %s ", kind)))
	w.Write([]byte(command))
	w.Write([]byte("\nRequest-Id: "))
	w.Write([]byte(fmt.Sprintf("%d", requestID)))
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		w.Write([]byte(fmt.Sprintf("\nContent-Length: %d", len(jsonBody))))
		w.Write([]byte("\nContent-Type: application/json"))
		w.Write([]byte("\n\n"))
		w.Write(jsonBody)
	} else {
		w.Write([]byte("\n\n"))
	}
}
