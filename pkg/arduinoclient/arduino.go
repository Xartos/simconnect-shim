package arduinoclient

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/tarm/serial"

	"go.uber.org/zap"
)

type ArduinoListener interface {
	Update(id int, eventType int, value int32)
}

func NewArduinoClient(dev string, listener ArduinoListener, logger *zap.Logger) (*ArduinoClient, error) {
	c := &serial.Config{
		Name: dev,
		Baud: 115200,
		Size: 8,
	}

	return &ArduinoClient{c, listener, make(chan eventData), make(chan bool), logger}, nil
}

type eventData struct {
	id        byte
	eventType byte
	data      string
}

type ArduinoClient struct {
	config     *serial.Config
	listener   ArduinoListener
	writeQueue chan eventData
	stopSignal chan bool
	logger     *zap.Logger
}

func (a *ArduinoClient) SendEvent(id byte, eventType byte, data string) error {
	if len(data) > 254 {
		return fmt.Errorf("")
	}

	a.writeQueue <- eventData{id, eventType, data}

	return nil
}

func (a *ArduinoClient) Connect() error {
	a.logger.Debug("arduino_connect")
	stream, err := serial.OpenPort(a.config)
	if err != nil {
		return fmt.Errorf("arduino connect: %s", err)
	}

	scanner := bufio.NewScanner(stream)
	err = scanner.Err()
	if err != nil {
		a.logger.Fatal("scanner_new", zap.Error(err))
	}

	startsGot := 0

	for startsGot < 2 {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil {
				a.logger.Fatal("scanner_new", zap.Error(err))
			}
		}

		id := scanner.Bytes()
		a.logger.Debug("reading_init_id", zap.Binary("data", id), zap.Int("len", len(id)))
		if len(id) == 0 {
			startsGot += 1
		} else {
			startsGot = 0
		}
	}

	gotAllIDs := false
	var allIDs []byte

	for !gotAllIDs {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil {
				a.logger.Fatal("scanner_new", zap.Error(err))
			}
		}

		id := scanner.Bytes()
		a.logger.Debug("reading_init_id", zap.Binary("data", id), zap.Int("len", len(id)))

		if len(id) == 1 {
			newID := id[0]
			allIDs = append(allIDs, newID)
			a.logger.Info("device_id", zap.Int8("id", int8(newID)))
		} else if len(id) == 0 {
			gotAllIDs = true
			continue
		} else {
			a.logger.Fatal("initializing_ids", zap.Binary("id", id))
		}
	}

	gotAllEvents := false
	var allEvents []byte

	for !gotAllEvents {
		if !scanner.Scan() {
			err := scanner.Err()
			if err != nil {
				a.logger.Fatal("scanner_new", zap.Error(err))
			}
		}

		event := scanner.Bytes()
		a.logger.Debug("reading_init_event", zap.Binary("data", event), zap.Int("len", len(event)))

		if len(event) > 1 {
			newID := event[0]
			name := event[1:]
			allEvents = append(allEvents, newID)
			a.logger.Info("device_id", zap.Int8("id", int8(newID)), zap.ByteString("name", name))
		} else if len(event) == 0 {
			gotAllEvents = true
			continue
		} else {
			a.logger.Fatal("initializing_events", zap.Binary("event", event))
		}
	}

	go a.arduinoReadWriter(stream)

	return nil
}

func (a *ArduinoClient) Disconnect() {
	a.stopSignal <- true
}

func arduinoLineReader(stream *serial.Port, writeChannel chan []byte, stopChan chan bool, logger *zap.Logger) {
	isStop := false

	// for !isStop {

	// 	scanner := bufio.NewScanner(stream)
	// 	err := scanner.Err()
	// 	if err != nil {
	// 		logger.Fatal("scanner_new", zap.Error(err))
	// 	}

	// 	for scanner.Scan() {
	// 		writeChannel <- scanner.Bytes()

	// 		select {
	// 		case stop := <-stopChan:
	// 			logger.Debug("reader_stop")
	// 			isStop = stop
	// 			break
	// 		default:
	// 			break
	// 		}
	// 	}

	// 	err = scanner.Err()
	// 	if err != nil {
	// 		logger.Error("scanner_new", zap.Error(err))
	// 		continue
	// 	}

	// }
	buf := make([]byte, 1)
	fullBuf := make([]byte, 256)

	for !isStop {
		n, err := stream.Read(buf)
		if err != nil {
			logger.Fatal("reading_stream", zap.Error(err))
		} else if n != 1 {
			logger.Fatal("reading_stream_n", zap.Int("n", n))
		}

		c := buf[0]
		if c == '\n' {
			writeChannel <- fullBuf
			fullBuf = make([]byte, 256)

			select {
			case stop := <-stopChan:
				logger.Debug("reader_stop")
				isStop = stop
			default:
				continue
			}
			continue
		}

		fullBuf = append(fullBuf, c)

	}

}

func (a *ArduinoClient) arduinoReadWriter(stream *serial.Port) {
	//defer stream.Close()
	isStop := false

	readQueue := make(chan []byte)
	readStop := make(chan bool, 1)
	go arduinoLineReader(stream, readQueue, readStop, a.logger)

	for !isStop {
		select {
		case write := <-a.writeQueue:
			a.logger.Debug("arduino_write",
				zap.Int8("id", int8(write.id)),
				zap.Int8("event_type", int8(write.eventType)),
				zap.Int8("data_len", int8(len(write.data))),
				zap.String("data", write.data))

			toSend := append([]byte{write.id, write.eventType, byte(len(write.data))}, []byte(write.data)...)
			toSend = append(toSend, '\n')

			stream.Write(toSend)
			stream.Flush()
		case stop := <-a.stopSignal:
			a.logger.Debug("arduino_stop")
			isStop = stop
			continue
		case read := <-readQueue:
			a.logger.Debug("arduino_read", zap.Binary("value", read))
			if len(read) > 3 {
				id := int(read[0])
				eventType := int(read[1])
				dataLen := int(read[2])
				a.logger.Debug("arduino_got_values",
					zap.Int("id", id),
					zap.Int("event_type", eventType),
					zap.Int("data_len", dataLen),
					zap.Int("len_of_data", len(read)))
				if dataLen+3 == len(read) {
					a.logger.Warn("protocol_decode_data", zap.ByteString("data", read))
					//data := binary.LittleEndian.Int32(read[3 : 3+dataLen])
					data := bytes.NewReader(read[3 : 3+dataLen])
					var value int32
					binary.Read(data, binary.BigEndian, &value)
					//	Varint(read[3 : 3+dataLen])
					//	var data int32
					//	data = (read[3] << 24) | (read[4] << 16) | (read[5] << 8) | read[6]
					a.listener.Update(id, eventType, value)

					//if n > 0 {
					//	a.listener.Update(id, eventType, int32(data))
					//} else {
					//	a.logger.Fatal("protocol_decode_data", zap.ByteString("data", read), zap.Int("n", n))
					//}
				} else {
					a.logger.Fatal("protocol_data_len", zap.ByteString("data", read))
				}
			} else {
				a.logger.Fatal("protocol_not_enough_data", zap.ByteString("data", read))
			}
		}
	}
	readStop <- true
	close(readQueue)
	stream.Close()
	a.logger.Info("arduino_disconnect")
}
