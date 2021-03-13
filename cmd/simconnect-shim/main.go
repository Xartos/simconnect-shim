package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"../../pkg/arduinoclient"
	//	"../../pkg/simconnector"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger
var gArduino *arduinoclient.ArduinoClient

//var gSimConnect *simconnector.SimConnectorClient

//freq := 100.125

type ArduinoListener struct{}

func (al *ArduinoListener) Update(id int, eventType int, value int32) {
	logger.Debug("arduino_listener_update", zap.Int("id", id), zap.Int("event_type", eventType), zap.Int32("value", value))
	//mute, err := gPulseClient.GetMute()
	//if err != nil {
	//	logger.Error("getting_mute", zap.Error(err))
	//	return
	//}
	//gPulseClient.Mute(!mute)
}

// type SimConnectorListener struct{}
//
// func (sl *SimConnectorListener) Update(id int, report simconnector.Report) {
// 	logger.Debug("simconnect_listener_update", zap.ByteString("Title", report.Title[:]), zap.Float64("Altitude", report.Altitude))
// }

func main() {
	setupLogger()

	arduino, err := arduinoclient.NewArduinoClient("/dev/ttyACM0", &ArduinoListener{}, logger)
	if err != nil {
		logger.Fatal("arduino_create", zap.Error(err))
	}

	arduino.Connect()
	gArduino = arduino

	//sc, err := simconnector.NewSimConnectorClient(&SimConnectorListener{}, logger)
	//if err != nil {
	//	logger.Fatal("simconnect_create", zap.Error(err))
	//}

	//sc.StartRead()
	//gSimConnect = sc

	// gArduino.SendEvent(2, 0, "9000")
	// gArduino.SendEvent(3, 0, "121.125")

	// //time.Sleep(5 * time.Second)

	// gArduino.SendEvent(2, 0, "FL350")
	// gArduino.SendEvent(3, 0, "117.100")

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)

	_ = <-sigc

	logger.Info("signal_caught_exit")

	arduino.Disconnect()
	//sc.Stop()

	time.Sleep(1 * time.Second)
}

func setupLogger() error {
	if logger != nil {
		panic("logger already setup")
	}

	config := zap.NewDevelopmentConfig()

	logLevel := "debug"

	if err := config.Level.UnmarshalText([]byte(logLevel)); err != nil {
		return err
	}

	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.DisableStacktrace = true

	var err error
	logger, err = config.Build()
	if err != nil {
		return fmt.Errorf("error building logger: %s", err)
	}

	return nil
}
