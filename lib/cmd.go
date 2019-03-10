package roonlib

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/olebedev/emitter"
	"github.com/sirupsen/logrus"
)

const (
	roonServiceID = "00720724-5143-4a9b-abac-0e50cba674bb"

	serviceRegistry  = "com.roonlabs.registry:1"
	serviceTransport = "com.roonlabs.transport:2"
	serviceStatus    = "com.roonlabs.status:1"
	servicePairing   = "com.roonlabs.pairing:1"
	servicePing      = "com.roonlabs.ping:1"
	serviceImage     = "com.roonlabs.image:1"
	serviceBrowse    = "com.roonlabs.browse:1"
	serviceSettings  = "com.roonlabs.settings:1"
	controlVolume    = "com.roonlabs.volumecontrol:1"
	controlSource    = "com.roonlabs.sourcecontrol:1"
)

type subType string

const (
	SubTypeZones subType = "zones"
)

func (t subType) Key() string {
	return string(t)
}

func (t subType) Service() string {
	switch t {
	case SubTypeZones:
		return serviceTransport
	default:
		logrus.Panicf("unknown subType '%s'", t)
		return ""
	}
}

func hasString(items []string, s string) bool {
	for _, item := range items {
		if item == s {
			return true
		}
	}
	return false
}

func filterZonesChange(change *ZonesChangedT) *ZonesChangedT {
	if len(Cfg.Roon.Zones) == 0 {
		return change
	}
	newChange := &ZonesChangedT{
		Zones: []*ZoneT{},
		Seek:  []*ZoneSeekChangedT{},
	}
	if change.Seek != nil {
		for _, seek := range change.Seek {
			if hasString(Cfg.Roon.Zones, seek.ID) {
				newChange.Seek = append(newChange.Seek, seek)
			}
		}
	}
	if change.Zones != nil {
		for _, zone := range change.Zones {
			if hasString(Cfg.Roon.Zones, zone.ID) {
				newChange.Zones = append(newChange.Zones, zone)
			}
		}
	}
	if len(newChange.Seek) == 0 {
		newChange.Seek = nil
	}
	if len(newChange.Zones) == 0 {
		newChange.Zones = nil
	}
	return newChange
}

func (t subType) ParseChange(data []byte) (interface{}, error) {
	switch t {
	case SubTypeZones:
		var zonesChange ZonesChangedT
		err := json.Unmarshal(data, &zonesChange)
		return filterZonesChange(&zonesChange), err
	default:
		logrus.Panicf("unknown subType '%s'", t)
		return nil, nil
	}
}

func (t subType) Endpoint() string {
	return string(t)
}

// RoonExtension .
type RoonExtension struct {
	DisplayName    string
	ExtensionID    string
	DisplayVersion string
	Publisher      string
	Email          string
}

type regInfo struct {
	DisplayName      string   `json:"display_name"`
	ExtensionID      string   `json:"extension_id"`
	DisplayVersion   string   `json:"display_version"`
	Publisher        string   `json:"publisher"`
	Email            string   `json:"email"`
	RequiredServices []string `json:"required_services"`
	ProvidedServices []string `json:"provided_services"`
	Token            string   `json:"token,omitempty"`
}

func (ext *RoonExtension) regInfo() json.RawMessage {
	regInfo := &regInfo{
		DisplayName:    ext.DisplayName,
		ExtensionID:    ext.ExtensionID,
		DisplayVersion: ext.DisplayVersion,
		Publisher:      ext.Publisher,
		Email:          ext.Email,
	}
	data, err := json.Marshal(regInfo)
	if err != nil {
		panic(err)
	}
	return data
}

type soodMessage struct {
	From   *net.UDPAddr
	Fields map[string]string
}

func parseMessage(data []byte, length int) map[string]string {
	// SOOD 2
	if bytes.Compare(data[0:5], []byte{0x53, 0x4F, 0x4F, 0x44, 0x02}) != 0 {
		return nil
	}
	msgType := string(data[5:6])
	if msgType != "R" {
		return nil
	}
	fields := make(map[string]string)
	body := data[6:]
	offset := 0
	for offset < length {
		var nameLen uint8
		err := binary.Read(bytes.NewReader(body[offset:]), binary.BigEndian, &nameLen)
		if err != nil {
			panic(err)
		}
		offset++
		if nameLen == 0 {
			continue
		}
		name := body[offset : offset+int(nameLen)]
		offset += int(nameLen)
		var valueLen uint16
		err = binary.Read(bytes.NewReader(body[offset:]), binary.BigEndian, &valueLen)
		if err != nil {
			panic(err)
		}
		offset += 2
		if valueLen == 0 || valueLen == 65535 {
			continue
		}
		value := body[offset : offset+int(valueLen)]
		fields[string(name)] = string(value)
		offset += int(valueLen)
	}
	serviceID := fields["service_id"]
	if serviceID != roonServiceID {
		return nil
	}
	return fields
}

const maxDatagramSize = 65535

func multicastListen(ready chan<- *net.UDPConn, messages chan<- *soodMessage) {
	addr, err := net.ResolveUDPAddr("udp", "239.255.90.90:9003")
	if err != nil {
		panic(err)
	}
	socket, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}
	defer socket.Close()
	socket.SetReadBuffer(maxDatagramSize)
	ready <- socket
	for {
		data := make([]byte, maxDatagramSize)
		length, addr, err := socket.ReadFromUDP(data)
		if err != nil {
			panic(err)
		}
		fields := parseMessage(data, length)
		if fields != nil {
			messages <- &soodMessage{
				From:   addr,
				Fields: fields,
			}
		}
	}
}

type soodQuery struct {
	queryServiceID string
	uuid           string
}

func newSoodQuery() *soodQuery {
	soodUUID, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}
	return &soodQuery{
		queryServiceID: roonServiceID,
		uuid:           soodUUID.String(),
	}
}

func (query *soodQuery) writeHeader(out io.Writer) {
	out.Write([]byte{0x53, 0x4F, 0x4F, 0x44, 0x02, 0x51}) // SOOD 2 Q
}

func (query *soodQuery) writeName(out io.Writer, name string) {
	binary.Write(out, binary.BigEndian, uint8(len(name)))
	out.Write([]byte(name))
}

func (query *soodQuery) writeValue(out io.Writer, value string) {
	binary.Write(out, binary.BigEndian, uint16(len(value)))
	out.Write([]byte(value))
}

func (query *soodQuery) write(out io.Writer) {
	query.writeHeader(out)
	query.writeName(out, "query_service_id")
	query.writeValue(out, query.queryServiceID)
	query.writeName(out, "_tid")
	query.writeName(out, query.uuid)
}

func multicastClient(c *net.UDPConn) {
	roonAddr, err := net.ResolveUDPAddr("udp", "239.255.90.90:9003")
	if err != nil {
		panic(err)
	}

	query := newSoodQuery()
	buffer := bytes.NewBuffer([]byte{})
	query.write(buffer)

	written, err := c.WriteToUDP(buffer.Bytes(), roonAddr)
	if err != nil {
		panic(err)
	}
	if written != buffer.Len() {
		panic(fmt.Errorf("only written %d of %d", written, buffer.Len()))
	}
}

func readPump(conn *websocket.Conn, messages chan<- []byte) {
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			panic(err)
		}
		if messageType != websocket.BinaryMessage {
			logrus.Panicf("unsupported message type %d\n", messageType)
		}
		messages <- message
	}
}

type wsClient struct {
	ip              net.IP
	port            int64
	Events          *emitter.Emitter
	conn            *websocket.Conn
	requestID       int
	subKey          int
	mutex           *sync.Mutex
	subs            map[int]subType
	zoneStatus      map[string]*zoneStatus
	sourceRequestID int
	volumeRequestID int
}

func NewWsClient(ip net.IP, port int64) *wsClient {
	return &wsClient{
		ip:              ip,
		port:            port,
		Events:          &emitter.Emitter{},
		conn:            nil,
		requestID:       10,
		subKey:          30,
		mutex:           &sync.Mutex{},
		subs:            make(map[int]subType),
		zoneStatus:      make(map[string]*zoneStatus),
		sourceRequestID: -1,
		volumeRequestID: -1,
	}
}

func (ws *wsClient) Connect() error {
	u := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%d", ws.ip.String(), ws.port),
		Path:   "/api",
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	ws.conn = conn
	go ws.readPump()
	logrus.Infof("websocket connected to %s", u.String())
	return nil
}

func (ws *wsClient) readPump() {
	for {
		messageType, message, err := ws.conn.ReadMessage()
		if err != nil {
			panic(err)
		}
		if messageType != websocket.BinaryMessage {
			logrus.Panicf("unsupported message type %d\n", messageType)
		}
		requestID, head1, head2, data, err := parseMOOResponse(message)
		if err != nil {
			panic(err)
		}
		if head1 == mooContinue && head2 == mooChanged {
			ws.mutex.Lock()
			sub, ok := ws.subs[requestID]
			ws.mutex.Unlock()
			if !ok {
				logrus.Panicf("sub not found for request=%d", requestID)
			}
			changeData, err := sub.ParseChange(data)
			if err != nil {
				panic(err)
			}
			ws.Events.Emit(sub.Key(), changeData)
		} else if head1 == mooContinue || (head1 == mooComplete && head2 == mooSuccess) {
			ws.Events.Emit(fmt.Sprintf("moo-%d", requestID), data)
		} else if head1 == mooRequest && head2 == fmt.Sprintf("%s/subscribe_controls", controlVolume) {
			logrus.Debugf("subscsribe to volume %d", requestID)
			ws.mutex.Lock()
			ws.volumeRequestID = requestID
			ws.mutex.Unlock()
		} else if head1 == mooRequest && head2 == fmt.Sprintf("%s/subscribe_controls", controlSource) {
			logrus.Debugf("subscsribe to source %d", requestID)
			ws.mutex.Lock()
			ws.sourceRequestID = requestID
			ws.mutex.Unlock()
		} else {
			logrus.WithField("head1", head1).WithField("head2", head2).Warnf("unknown head combo %s", data)
		}
	}
}

func (ws *wsClient) sendRequest(path string, data interface{}) (int, error) {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	ws.requestID++
	requestID := ws.requestID
	buffer := bytes.NewBuffer([]byte{})
	createMooMessage(buffer, mooRequest, ws.requestID, path, data)
	return requestID, ws.conn.WriteMessage(websocket.BinaryMessage, buffer.Bytes())
}

func (ws *wsClient) waitForResponse(requestID int) []byte {
	event := <-ws.Events.Once(fmt.Sprintf("moo-%d", requestID))
	return event.Args[0].([]byte)
}

func (ws *wsClient) makeRequest(path string, data interface{}, into interface{}) (int, error) {
	requestID, err := ws.sendRequest(path, data)
	if err != nil {
		return -1, err
	}
	response := ws.waitForResponse(requestID)
	return requestID, json.Unmarshal(response, into)
}

func (ws *wsClient) subscribe(sub subType, extra map[string]interface{}, into interface{}) error {
	ws.mutex.Lock()
	ws.subKey++
	subKey := ws.subKey
	ws.mutex.Unlock()

	data := map[string]interface{}{
		"subscription_key": subKey,
	}
	for key, value := range extra {
		data[key] = value
	}
	requestID, err := ws.makeRequest(fmt.Sprintf("%s/subscribe_%s", sub.Service(), sub.Endpoint()), data, &into)
	if err != nil {
		return err
	}

	ws.mutex.Lock()
	ws.subs[requestID] = sub
	ws.mutex.Unlock()

	return nil
}

func FindRoon() (net.IP, int64) {
	if len(Cfg.Roon.IP) > 0 && Cfg.Roon.Port > 0 {
		return net.ParseIP(Cfg.Roon.IP), Cfg.Roon.Port
	}

	logrus.Debug("looking for roon server")
	ready := make(chan *net.UDPConn)
	messages := make(chan *soodMessage)
	go multicastListen(ready, messages)
	go func() {
		addr := <-ready
		for {
			multicastClient(addr)
			time.Sleep(time.Second * 5)
		}
	}()
	knownServers := make(map[string]bool)
	for msg := range messages {
		serverID := msg.Fields["unique_id"]
		_, found := knownServers[serverID]
		if !found {
			knownServers[serverID] = true
			port, err := strconv.ParseInt(msg.Fields["http_port"], 10, 0)
			if err != nil {
				panic(err)
			}
			return msg.From.IP, port
		}
	}
	panic("cant find roon")
}

type zoneStatus struct {
	Name      string  `json:"-"`
	OutputID  string  `json:"-"`
	MaxVolume int64   `json:"-"`
	State     string  `json:"state"`
	Title     string  `json:"title"`
	Length    int64   `json:"length"`
	Seek      int64   `json:"seek"`
	Volume    float64 `json:"volume"`
}

func (ws *wsClient) ZoneStatusReducer() {
	status := make(map[string]*zoneStatus)
	for ev := range ws.Events.On(SubTypeZones.Key()) {
		changedZones := []string{}
		change := ev.Args[0].(*ZonesChangedT)

		if change.Zones != nil {
			status = make(map[string]*zoneStatus)
			for _, zoneChange := range change.Zones {
				vol := zoneChange.Outputs[0].Volume

				zoneSt := &zoneStatus{
					Name:      zoneChange.Name,
					State:     zoneChange.State,
					OutputID:  zoneChange.Outputs[0].ID,
					Volume:    float64(vol.Value) / float64(vol.HardLimitMax),
					MaxVolume: vol.HardLimitMax,
				}

				if zoneChange.NowPlaying != nil {
					zoneSt.Length = zoneChange.NowPlaying.Length
					zoneSt.Seek = zoneChange.NowPlaying.Seek
					zoneSt.Title = zoneChange.NowPlaying.OneLine.Line1
				}

				status[zoneChange.ID] = zoneSt
				if !hasString(changedZones, zoneChange.ID) {
					changedZones = append(changedZones, zoneChange.ID)
				}
			}
		}

		if change.Seek != nil {
			for _, zoneSeek := range change.Seek {
				zoneStatus, found := status[zoneSeek.ID]
				if found {
					zoneStatus.Seek = zoneSeek.Seek
					if !hasString(changedZones, zoneSeek.ID) {
						changedZones = append(changedZones, zoneSeek.ID)
					}
				}
			}
		}

		ws.mutex.Lock()
		ws.zoneStatus = status
		ws.mutex.Unlock()
	}
}
