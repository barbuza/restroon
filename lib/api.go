package roonlib

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

type registerT struct {
	CoreID         string `json:"core_id"`
	DisplayName    string `json:"display_name"`
	DisplayVersion string `json:"display_version"`
	Token          string `json:"token"`
}

func (ws *wsClient) MakeRegisterRequest() (*registerT, error) {
	info := &regInfo{
		DisplayName:    "github.com/barbuza/restroon",
		DisplayVersion: "0.0.1",
		Email:          "barbuzaster@gmail.com",
		Publisher:      "Victor Kotseruba",
		RequiredServices: []string{
			serviceTransport,
			serviceBrowse,
		},
		ProvidedServices: []string{
			// controlSource,
			// controlVolume,
		},
	}

	if len(Cfg.Roon.Token) > 0 {
		info.Token = Cfg.Roon.Token
	} else {
		logrus.Warn("token not provided, authorize roon extension")
	}

	var resp registerT
	_, err := ws.makeRequest(fmt.Sprintf("%s/register", serviceRegistry), info, &resp)
	return &resp, err
}

type VolumeT struct {
	Min          int64 `json:"min"`
	Max          int64 `json:"max"`
	Value        int64 `json:"value"`
	Step         int64 `json:"step"`
	IsMuted      bool  `json:"is_muted"`
	HardLimitMin int64 `json:"hard_limit_min"`
	HardLimitMax int64 `json:"hard_limit_max"`
	SoftLimit    int64 `json:"soft_limit"`
}

type OutputT struct {
	ID     string   `json:"output_id"`
	Name   string   `json:"display_name"`
	Volume *VolumeT `json:"volume"`
}

type SettingsT struct {
	AutoRadio bool `json:"auto_radio"`
}

type OneLineT struct {
	Line1 string `json:"line1"`
}

type TwoLineT struct {
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
}

type ThreeLineT struct {
	Line1 string `json:"line1"`
	Line2 string `json:"line2"`
	Line3 string `json:"line3"`
}

type NowPlayingT struct {
	Seek      int64       `json:"seek_position"`
	Length    int64       `json:"length"`
	OneLine   *OneLineT   `json:"one_line"`
	TwoLine   *TwoLineT   `json:"two_line"`
	ThreeLine *ThreeLineT `json:"three_line"`
}

type ZoneT struct {
	ID             string       `json:"zone_id"`
	Name           string       `json:"display_name"`
	Outputs        []*OutputT   `json:"outputs"`
	State          string       `json:"state"`
	IsNextAllowed  bool         `json:"is_next_allowed"`
	IsPauseAllowed bool         `json:"is_pause_allowed"`
	IsPlayAllowed  bool         `json:"is_play_allowed"`
	IsSeekAllowed  bool         `json:"is_seek_allowed"`
	Settings       *SettingsT   `json:"settings"`
	NowPlaying     *NowPlayingT `json:"now_playing"`
}

type ZonesT struct {
	Zones []*ZoneT `json:"zones"`
}

type ZoneSeekChangedT struct {
	ID   string `json:"zone_id"`
	Seek int64  `json:"seek_position"`
}

type ZonesChangedT struct {
	Zones []*ZoneT            `json:"zones_changed"`
	Seek  []*ZoneSeekChangedT `json:"zones_seek_changed"`
}

func (ws *wsClient) ZonesSubscribe() error {
	var response ZonesT
	err := ws.subscribe(SubTypeZones, map[string]interface{}{}, &response)
	if err != nil {
		return err
	}
	change := filterZonesChange(&ZonesChangedT{
		Zones: response.Zones,
	})
	ws.Events.Emit(SubTypeZones.Key(), change)
	return nil
}
