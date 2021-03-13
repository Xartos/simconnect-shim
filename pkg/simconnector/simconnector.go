package simconnector

import (
	"fmt"
	"reflect"
	"time"

	"github.com/lian/msfs2020-go/simconnect"

	"go.uber.org/zap"
)

type SimConnectorListener interface {
	Update(id int, value Report)
}

func NewSimConnectorClient(
	listener SimConnectorListener,
	logger *zap.Logger,
) (*SimConnectorClient, error) {
	s, err := simconnect.New("Arduino connect Client")
	if err != nil {
		return nil, fmt.Errorf("simconnect_new: %s", err)
	}

	events := Events{
		ToggleNavLights: s.GetEventID(),
		ToggleAPMaster:  s.GetEventID(),
		ComWholeInc:     s.GetEventID(),
		ComWholeDec:     s.GetEventID(),
		ComFractInc:     s.GetEventID(),
		ComFractDec:     s.GetEventID(),
		ComSwap:         s.GetEventID(),
		ComTwoWholeInc:  s.GetEventID(),
		ComTwoWholeDec:  s.GetEventID(),
		ComTwoFractInc:  s.GetEventID(),
		ComTwoFractDec:  s.GetEventID(),
		ComTwoSwap:      s.GetEventID(),
	}

	fields := reflect.TypeOf(events)
	values := reflect.ValueOf(events)

	num := fields.NumField()

	for i := 0; i < num; i++ {
		field := fields.Field(i)
		value := values.Field(i)

		logger.Debug("map_client_event_to_sim_event",
			zap.String("name", field.Name),
			zap.String("tag", field.Tag.Get("name")),
			zap.Uint64("value", value.Uint()),
		)

		err = s.MapClientEventToSimEvent(simconnect.DWORD(value.Uint()), field.Tag.Get("name"))
		if err != nil {
			panic(err)
		}
	}

	return &SimConnectorClient{
		listener:   listener,
		sc:         s,
		events:     events,
		stopSignal: make(chan bool),
		logger:     logger,
	}, nil
}

func (s *SimConnectorClient) StartRead() {
	go s.simconnectReader()
}

func (s *SimConnectorClient) Stop() error {
	s.stopSignal <- true
	return s.sc.Close()
}

func (s *SimConnectorClient) simconnectReader() {

	report := &Report{}
	s.sc.RegisterDataDefinition(report)
	report.RequestData(s.sc)

	isStop := false

	for !isStop {

		select {
		case stop := <-s.stopSignal:
			s.logger.Debug("reader_stop")
			isStop = stop
			continue
		default:
			// NOOP
		}

		ppData, r1, err := s.sc.GetNextDispatch()

		if r1 < 0 {
			if uint32(r1) == simconnect.E_FAIL {
				continue
			} else {
				s.logger.Panic("GetNextDispatch", zap.Uint32("r1", uint32(r1)), zap.Error(err))
			}
		}

		recvInfo := *(*simconnect.Recv)(ppData)

		switch recvInfo.ID {
		case simconnect.RECV_ID_EXCEPTION:
			recvErr := *(*simconnect.RecvException)(ppData)
			s.logger.Error("SIMCONNECT_RECV_ID_EXCEPTION", zap.String("err", fmt.Sprintf("%#v", recvErr)))

		case simconnect.RECV_ID_OPEN:
			recvOpen := *(*simconnect.RecvOpen)(ppData)
			s.logger.Info("SIMCONNECT_RECV_ID_OPEN", zap.String("err", fmt.Sprintf("%s", recvOpen.ApplicationName)))
			//spew.Dump(recvOpen)

		case simconnect.RECV_ID_EVENT:
			recvEvent := *(*simconnect.RecvEvent)(ppData)
			s.logger.Debug("SIMCONNECT_RECV_ID_EVENT")
			//spew.Dump(recvEvent)

			switch recvEvent.EventID {
			//case eventSimStartID:
			//	fmt.Println("SimStart Event")
			default:
				s.logger.Warn("unknown SIMCONNECT_RECV_ID_EVENT", zap.Int("eventID", int(recvEvent.EventID)))
			}

		case simconnect.RECV_ID_SIMOBJECT_DATA_BYTYPE:
			recvData := *(*simconnect.RecvSimobjectDataByType)(ppData)
			s.logger.Debug("SIMCONNECT_RECV_SIMOBJECT_DATA_BYTYPE")

			switch recvData.RequestID {
			case s.sc.DefineMap["Report"]:
				report := (*Report)(ppData)
				s.logger.Debug("got_report",
					zap.String("title", fmt.Sprintf("%s", report.Title[:])),
					zap.Int("altitude", int(report.Altitude)),
				)
				s.listener.Update(0, *report)
				report.RequestData(s.sc)
			}

		default:
			fmt.Println("recvInfo.dwID unknown", recvInfo.ID)
		}

		time.Sleep(200 * time.Millisecond)
	}
}

type SimConnectorClient struct {
	listener   SimConnectorListener
	sc         *simconnect.SimConnect
	events     Events
	stopSignal chan bool
	logger     *zap.Logger
}

func (s *SimConnectorClient) TransmitEvent(event Event) error {
	switch event {
	case ToggleNavLights:
		s.logger.Info("toggle_nav_light")
		return s.sc.TransmitClientID(s.events.ToggleNavLights, 0)
	default:
		s.logger.Error("unknown_transmit_event", zap.Int("event", int(event)))
		return fmt.Errorf("unknown_transmit_event: %d", event)
	}
}

type Event int

const (
	ToggleNavLights Event = iota // Toggle navigation lights
	ToggleAPMaster               // Toggle AP
	ComWholeInc                  // Com 1 whole increase
	ComWholeDec                  // Com 1 whole decrease
	ComFractInc                  // Com 1 frac increase
	ComFractDec                  // Com 1 frac decrease
	ComSwap
	ComTwoWholeInc
	ComTwoWholeDec
	ComTwoFractInc
	ComTwoFractDec
	ComTwoSwap
)

type Events struct {
	ToggleNavLights simconnect.DWORD `name:"TOGGLE_NAV_LIGHTS"`
	ToggleAPMaster  simconnect.DWORD `name:"AP_MASTER"`
	ComWholeInc     simconnect.DWORD `name:"COM_RADIO_WHOLE_INC"`
	ComWholeDec     simconnect.DWORD `name:"COM_RADIO_WHOLE_DEC"`
	ComFractInc     simconnect.DWORD `name:"COM_RADIO_FRACT_INC"`
	ComFractDec     simconnect.DWORD `name:"COM_RADIO_FRACT_DEC"`
	ComSwap         simconnect.DWORD `name:"COM_STBY_RADIO_SWAP"`
	ComTwoWholeInc  simconnect.DWORD `name:"COM2_RADIO_WHOLE_INC"`
	ComTwoWholeDec  simconnect.DWORD `name:"COM2_RADIO_WHOLE_DEC"`
	ComTwoFractInc  simconnect.DWORD `name:"COM2_RADIO_FRACT_INC"`
	ComTwoFractDec  simconnect.DWORD `name:"COM2_RADIO_FRACT_DEC"`
	ComTwoSwap      simconnect.DWORD `name:"COM2_RADIO_SWAP"`
}

type Report struct {
	simconnect.RecvSimobjectDataByType
	Title    [256]byte `name:"TITLE"`
	Altitude float64   `name:"Plane Altitude" unit:"feet"`
}

func (r *Report) RequestData(s *simconnect.SimConnect) {
	defineID := s.GetDefineID(r)
	s.RequestDataOnSimObjectType(defineID, defineID, 0, simconnect.SIMOBJECT_TYPE_USER)
}

//func main() {
//	s, err := simconnect.New("Transmit Event")
//	if err != nil {
//		panic(err)
//	}
//	fmt.Println("Connected to Flight Simulator!")
//
//	events := &Events{
//		ToggleNavLights: s.GetEventID(),
//		ToggleAPMaster:  s.GetEventID(),
//		ComWholeDec:     s.GetEventID(),
//		ComFractDec:     s.GetEventID(),
//		ComSwap:         s.GetEventID(),
//		ComTwoWholeDec:  s.GetEventID(),
//		ComTwoFractDec:  s.GetEventID(),
//		ComTwoSwap:      s.GetEventID(),
//		ComSelect:       s.GetEventID(),
//		KeySelectTwo:    s.GetEventID(),
//		FreqSwap:        s.GetEventID(),
//	}
//
//	err = s.MapClientEventToSimEvent(events.ToggleNavLights, "TOGGLE_NAV_LIGHTS")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ToggleAPMaster, "AP_MASTER")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComWholeDec, "COM_RADIO_WHOLE_DEC")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComFractDec, "COM_RADIO_FRACT_DEC")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComSwap, "COM_STBY_RADIO_SWAP")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComTwoWholeDec, "COM2_RADIO_WHOLE_DEC")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComTwoFractDec, "COM2_RADIO_FRACT_DEC")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComTwoSwap, "COM2_RADIO_SWAP")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.ComSelect, "COM_RADIO")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.KeySelectTwo, "SELECT_2")
//	if err != nil {
//		panic(err)
//	}
//
//	err = s.MapClientEventToSimEvent(events.FreqSwap, "FREQUENCY_SWAP")
//	if err != nil {
//		panic(err)
//	}
//
//	for {
//
//		err = s.TransmitClientID(events.ComFractDec, 0)
//		if err != nil {
//			panic(err)
//		}
//
//		time.Sleep(2000 * time.Millisecond)
//
//		err = s.TransmitClientID(events.ComWholeDec, 0)
//		if err != nil {
//			panic(err)
//		}
//
//		err = s.TransmitClientID(events.ComSwap, 0)
//		if err != nil {
//			panic(err)
//		}
//
//		time.Sleep(2000 * time.Millisecond)
//
//		err = s.TransmitClientID(events.KeySelectTwo, 0)
//		if err != nil {
//			panic(err)
//		}
//		err = s.TransmitClientID(events.ComSelect, 0)
//		if err != nil {
//			panic(err)
//		}
//		err = s.TransmitClientID(events.KeySelectTwo, 0)
//		if err != nil {
//			panic(err)
//		}
//
//		time.Sleep(2000 * time.Millisecond)
//
//		err = s.TransmitClientID(events.ComTwoFractDec, 0)
//		if err != nil {
//			panic(err)
//		}
//
//		time.Sleep(2000 * time.Millisecond)
//
//		err = s.TransmitClientID(events.ComTwoWholeDec, 0)
//		if err != nil {
//			panic(err)
//		}
//
//		time.Sleep(2000 * time.Millisecond)
//
//		err = s.TransmitClientID(events.ComTwoSwap, 0)
//		if err != nil {
//			panic(err)
//		}
//	}
//
//	fmt.Println("close")
//
//	if err = s.Close(); err != nil {
//		panic(err)
//	}
//}
