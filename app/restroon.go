package main

import (
	"os"
	"time"

	roonlib "github.com/barbuza/restroon/lib"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

func main() {
	roonlib.ParseConfig()
	logrus.SetOutput(os.Stdout)

	switch roonlib.Cfg.Logging.Formatter {
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{})
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			PrettyPrint:     true,
			TimestampFormat: time.RFC3339Nano,
		})
	default:
		logrus.Panicf("unknown log formatter '%s'", roonlib.Cfg.Logging.Formatter)
	}

	switch roonlib.Cfg.Logging.Level {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.Panicf("unknown log level '%s'", roonlib.Cfg.Logging.Level)
	}

	configDump, err := yaml.Marshal(roonlib.Cfg)
	if err != nil {
		panic(err)
	}
	logrus.Debugf("config:\n%s", configDump)

	roonIP, roonPort := roonlib.FindRoon()

	ws := roonlib.NewWsClient(roonIP, roonPort)
	err = ws.Connect()
	if err != nil {
		panic(err)
	}

	registerResponse, err := ws.MakeRegisterRequest()
	if err != nil {
		panic(err)
	}
	if registerResponse.Token != roonlib.Cfg.Roon.Token {
		logrus.Warnf("new token %s authenticated", registerResponse.Token)
	}
	logrus.Infof("connected to core %s %s %s", registerResponse.CoreID, registerResponse.DisplayName, registerResponse.DisplayVersion)

	go ws.ZoneStatusReducer()
	go ws.ZonesSubscribe()

	if len(roonlib.Cfg.Rest.IP) > 0 && roonlib.Cfg.Rest.Port > 0 {
		rest := &roonlib.RestServer{
			Ws: ws,
		}
		go rest.Start()
	}

	for change := range ws.Events.On(roonlib.SubTypeZones.Key()) {
		data := change.Args[0].(*roonlib.ZonesChangedT)
		if err != nil {
			panic(err)
		}
		d, err := yaml.Marshal(data)
		if err != nil {
			panic(err)
		}
		logrus.Debugf("zones change:\n%s", d)
	}

	done := make(chan bool)
	<-done
}
