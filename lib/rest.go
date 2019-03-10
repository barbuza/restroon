package roonlib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/gorilla/mux"
)

type RestServer struct {
	Ws *wsClient
}

func (srv *RestServer) Start() {
	router := mux.NewRouter()

	router.
		Methods("GET").
		Path("/zone/by-id/{zoneID}/status").
		HandlerFunc(srv.zoneStatus)

	router.
		Methods("GET").
		Path("/zone/by-name/{zoneName}/status").
		HandlerFunc(srv.zoneStatus)

	router.
		Methods("PUT").
		Path("/zone/by-id/{zoneID}/volume/{volume}").
		HandlerFunc(srv.zoneVolume)

	router.
		Methods("PUT").
		Path("/zone/by-name/{zoneName}/volume/{volume}").
		HandlerFunc(srv.zoneVolume)

	restAddr := fmt.Sprintf("%s:%d", Cfg.Rest.IP, Cfg.Rest.Port)
	logrus.Infof("starting rest server on %s", restAddr)
	server := http.Server{
		Addr:    restAddr,
		Handler: router,
	}
	if err := server.ListenAndServe(); err != nil {
		panic(err)
	}
}

func (srv *RestServer) findZone(r *http.Request) (*zoneStatus, string) {
	vars := mux.Vars(r)
	zoneID := vars["zoneID"]
	zoneName := vars["zoneName"]

	var status zoneStatus
	var statusPtr *zoneStatus
	var search string
	srv.Ws.mutex.Lock()
	found := false
	if len(zoneID) > 0 {
		search = fmt.Sprintf("id=%s", zoneID)
		statusPtr, zoneFound := srv.Ws.zoneStatus[zoneID]
		if zoneFound {
			found = true
			status = *statusPtr
		}
	} else if len(zoneName) > 0 {
		search = fmt.Sprintf("name=%s", zoneName)
		for _, statusPtr = range srv.Ws.zoneStatus {
			if strings.ToLower(statusPtr.Name) == strings.ToLower(zoneName) {
				status = *statusPtr
				found = true
				break
			}
		}
	}
	srv.Ws.mutex.Unlock()

	if found {
		return &status, search
	}
	return nil, search
}

func (srv *RestServer) zoneVolume(w http.ResponseWriter, r *http.Request) {
	status, search := srv.findZone(r)
	if status == nil {
		w.WriteHeader(404)
		w.Write([]byte(fmt.Sprintf("zone %s not found", search)))
		return
	}

	vars := mux.Vars(r)
	volume, err := strconv.ParseFloat(vars["volume"], 64)
	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte("cant parse volume value"))
		return
	}

	if volume > 1 {
		volume = 1
	}

	targetVolume := int64(float64(status.MaxVolume) * volume)
	response := make(map[string]interface{})
	srv.Ws.makeRequest(fmt.Sprintf("%s/change_volume", serviceTransport), map[string]interface{}{
		"output_id": status.OutputID,
		"value":     targetVolume,
		"how":       "absolute",
	}, &response)
	w.WriteHeader(202)
}

func (srv *RestServer) zoneStatus(w http.ResponseWriter, r *http.Request) {
	status, search := srv.findZone(r)
	if status == nil {
		w.WriteHeader(404)
		w.Write([]byte(fmt.Sprintf("zone %s not found", search)))
		return
	}

	data, err := json.Marshal(status)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}
