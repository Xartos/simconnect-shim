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
		ComOneWholeInc:  s.GetEventID(),
		ComOneWholeDec:  s.GetEventID(),
		ComOneFractInc:  s.GetEventID(),
		ComOneFractDec:  s.GetEventID(),
		ComOneSwap:      s.GetEventID(),
		ComTwoWholeInc:  s.GetEventID(),
		ComTwoWholeDec:  s.GetEventID(),
		ComTwoFractInc:  s.GetEventID(),
		ComTwoFractDec:  s.GetEventID(),
		ComTwoSwap:      s.GetEventID(),
		NavOneWholeInc:  s.GetEventID(),
		NavOneWholeDec:  s.GetEventID(),
		NavOneFractInc:  s.GetEventID(),
		NavOneFractDec:  s.GetEventID(),
		NavOneSwap:      s.GetEventID(),
		NavTwoWholeInc:  s.GetEventID(),
		NavTwoWholeDec:  s.GetEventID(),
		NavTwoFractInc:  s.GetEventID(),
		NavTwoFractDec:  s.GetEventID(),
		NavTwoSwap:      s.GetEventID(),
		AltVarInc:       s.GetEventID(),
		AltVarDec:       s.GetEventID(),
		AltVarSet:       s.GetEventID(),
		HeadingBugInc:   s.GetEventID(),
		HeadingBugDec:   s.GetEventID(),
		HeadingBugSet:   s.GetEventID(),
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

		if err != nil {
				s.logger.Error("GetNextDispatch", zap.Error(err))
	  }


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
			s.logger.Panic("SIMCONNECT_RECV_ID_EXCEPTION", zap.String("err", fmt.Sprintf("%#v", recvErr)))

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
					//zap.String("title", fmt.Sprintf("%s", report.Title[:])),
					zap.Int("altitude", int(report.Altitude)),
					zap.Float64("com_one_active", report.ComOneActiveFrequency),
					zap.Float64("com_one_standby", report.ComOneStandbyFrequency),
					zap.Float64("com_two_active", report.ComTwoActiveFrequency),
					zap.Float64("com_two_standby", report.ComTwoStandbyFrequency),
					zap.Float64("nav_one_active", report.NavOneActiveFrequency),
					zap.Float64("nav_one_standby", report.NavOneStandbyFrequency),
					zap.Float64("nav_two_active", report.NavTwoActiveFrequency),
					zap.Float64("nav_two_standby", report.NavTwoStandbyFrequency),
					zap.Float64("heading", report.Heading),
					zap.Float64("headingMode", report.HeadingMode),
					zap.Float64("approachMode", report.ApproachMode),
					zap.Float64("altitudeMode", report.AltitudeMode),
					zap.Float64("verticalMode", report.VerticalMode),
					zap.Float64("speedMode", report.SpeedMode),
					zap.Float64("flcMode", report.FlcMode),
					zap.Float64("navMode", report.NavMode),
				)
				s.lastAlt = int(report.Altitude)
				s.lastHeading = report.Heading
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
	listener    SimConnectorListener
	sc          *simconnect.SimConnect
	events      Events
	stopSignal  chan bool
	lastHeading float64
	lastAlt     int
	logger      *zap.Logger
}

func (s *SimConnectorClient) TransmitEventData(event Event, data int) error {
	return s.TransmitEventDataDWORD(event, simconnect.DWORD(data))
}
func (s *SimConnectorClient) TransmitEventDataDWORD(event Event, data simconnect.DWORD) error {
	switch event {
	case ToggleNavLights:
		s.logger.Info("toggle_nav_light", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ToggleNavLights, data)
	case ComOneFractInc:
		s.logger.Info("com_one_fract_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComOneFractInc, data)
	case ComOneFractDec:
		s.logger.Info("com_one_fract_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComOneFractDec, data)
	case ComOneWholeInc:
		s.logger.Info("com_one_whole_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComOneWholeInc, data)
	case ComOneWholeDec:
		s.logger.Info("com_one_whole_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComOneWholeDec, data)
	case ComOneSwap:
		s.logger.Info("com_one_swap", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComOneSwap, data)
	case ComTwoFractInc:
		s.logger.Info("com_two_fract_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComTwoFractInc, data)
	case ComTwoFractDec:
		s.logger.Info("com_two_fract_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComTwoFractDec, data)
	case ComTwoWholeInc:
		s.logger.Info("com_two_whole_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComTwoWholeInc, data)
	case ComTwoWholeDec:
		s.logger.Info("com_two_whole_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComTwoWholeDec, data)
	case ComTwoSwap:
		s.logger.Info("com_two_swap", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.ComTwoSwap, data)
	case NavOneFractInc:
		s.logger.Info("nav_one_fract_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavOneFractInc, data)
	case NavOneFractDec:
		s.logger.Info("nav_one_fract_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavOneFractDec, data)
	case NavOneWholeInc:
		s.logger.Info("nav_one_whole_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavOneWholeInc, data)
	case NavOneWholeDec:
		s.logger.Info("nav_one_whole_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavOneWholeDec, data)
	case NavOneSwap:
		s.logger.Info("nav_one_swap", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavOneSwap, data)
	case NavTwoFractInc:
		s.logger.Info("nav_two_fract_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavTwoFractInc, data)
	case NavTwoFractDec:
		s.logger.Info("nav_two_fract_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavTwoFractDec, data)
	case NavTwoWholeInc:
		s.logger.Info("nav_two_whole_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavTwoWholeInc, data)
	case NavTwoWholeDec:
		s.logger.Info("nav_two_whole_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavTwoWholeDec, data)
	case NavTwoSwap:
		s.logger.Info("nav_two_swap", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.NavTwoSwap, data)
	case AltVarInc:
		s.logger.Info("ap_alt_var_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.AltVarInc, data)
	case AltVarDec:
		s.logger.Info("ap_alt_var_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.AltVarDec, data)
	case AltVarSet:
		s.logger.Info("ap_alt_var_set", zap.Uint32("data", uint32(data)))
		s.lastAlt = int(data)
		return s.sc.TransmitClientID(s.events.AltVarSet, data)
	case HeadingBugInc:
		s.logger.Info("heading_bug_inc", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.HeadingBugInc, data)
	case HeadingBugDec:
		s.logger.Info("heading_bug_dec", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.HeadingBugDec, data)
	case HeadingBugSet:
		s.logger.Info("heading_bug_set", zap.Uint32("data", uint32(data)))
		return s.sc.TransmitClientID(s.events.HeadingBugSet, data)
	default:
		s.logger.Error("unknown_transmit_event", zap.Int("event", int(event)))
		return fmt.Errorf("unknown_transmit_event: %d", event)
	}
}

func (s *SimConnectorClient) GetLastHeading() float64 {
	return s.lastHeading
}

func (s *SimConnectorClient) GetLastAlt() int {
	return s.lastAlt
}

func (s *SimConnectorClient) TransmitEvent(event Event) error {
	return s.TransmitEventDataDWORD(event, 0)
}

type Event int

const (
	ToggleNavLights Event = iota // Toggle navigation lights
	ToggleAPMaster               // Toggle AP
	ComOneWholeInc                  // Com 1 whole increase
	ComOneWholeDec                  // Com 1 whole decrease
	ComOneFractInc                  // Com 1 frac increase
	ComOneFractDec                  // Com 1 frac decrease
	ComOneSwap
	ComTwoWholeInc
	ComTwoWholeDec
	ComTwoFractInc
	ComTwoFractDec
	ComTwoSwap
	NavOneWholeInc                  // Nav 1 whole increase
	NavOneWholeDec                  // Nav 1 whole decrease
	NavOneFractInc                  // Nav 1 frac increase
	NavOneFractDec                  // Nav 1 frac decrease
	NavOneSwap
	NavTwoWholeInc                  // Nav 2 whole increase
	NavTwoWholeDec                  // Nav 2 whole decrease
	NavTwoFractInc                  // Nav 2 frac increase
	NavTwoFractDec                  // Nav 2 frac decrease
	NavTwoSwap
	AltVarInc
	AltVarDec
	AltVarSet
	HeadingBugInc
	HeadingBugDec
	HeadingBugSet
)

type Events struct {
	ToggleNavLights simconnect.DWORD `name:"TOGGLE_NAV_LIGHTS"`
	ToggleNavLights simconnect.DWORD `name:"TOGGLE_NAV_LIGHTS"`
	ToggleAPMaster  simconnect.DWORD `name:"AP_MASTER"`
	ComOneWholeInc  simconnect.DWORD `name:"COM_RADIO_WHOLE_INC"`
	ComOneWholeDec  simconnect.DWORD `name:"COM_RADIO_WHOLE_DEC"`
	ComOneFractInc  simconnect.DWORD `name:"COM_RADIO_FRACT_INC"`
	ComOneFractDec  simconnect.DWORD `name:"COM_RADIO_FRACT_DEC"`
	ComOneSwap      simconnect.DWORD `name:"COM_STBY_RADIO_SWAP"`
	ComTwoWholeInc  simconnect.DWORD `name:"COM2_RADIO_WHOLE_INC"`
	ComTwoWholeDec  simconnect.DWORD `name:"COM2_RADIO_WHOLE_DEC"`
	ComTwoFractInc  simconnect.DWORD `name:"COM2_RADIO_FRACT_INC"`
	ComTwoFractDec  simconnect.DWORD `name:"COM2_RADIO_FRACT_DEC"`
	ComTwoSwap      simconnect.DWORD `name:"COM2_RADIO_SWAP"`
	NavOneWholeInc  simconnect.DWORD `name:"NAV1_RADIO_WHOLE_INC"`
	NavOneWholeDec  simconnect.DWORD `name:"NAV1_RADIO_WHOLE_DEC"`
	NavOneFractInc  simconnect.DWORD `name:"NAV1_RADIO_FRACT_INC"`
	NavOneFractDec  simconnect.DWORD `name:"NAV1_RADIO_FRACT_DEC"`
	NavOneSwap      simconnect.DWORD `name:"NAV1_RADIO_SWAP"`
	NavTwoWholeInc  simconnect.DWORD `name:"NAV2_RADIO_WHOLE_INC"`
	NavTwoWholeDec  simconnect.DWORD `name:"NAV2_RADIO_WHOLE_DEC"`
	NavTwoFractInc  simconnect.DWORD `name:"NAV2_RADIO_FRACT_INC"`
	NavTwoFractDec  simconnect.DWORD `name:"NAV2_RADIO_FRACT_DEC"`
	NavTwoSwap      simconnect.DWORD `name:"NAV2_RADIO_SWAP"`
	AltVarInc       simconnect.DWORD `name:"AP_ALT_VAR_INC"`
	AltVarDec       simconnect.DWORD `name:"AP_ALT_VAR_DEC"`
	AltVarSet       simconnect.DWORD `name:"AP_ALT_VAR_SET_ENGLISH"`
	HeadingBugInc   simconnect.DWORD `name:"HEADING_BUG_INC"`
	HeadingBugDec   simconnect.DWORD `name:"HEADING_BUG_DEC"`
	HeadingBugSet   simconnect.DWORD `name:"HEADING_BUG_SET"`
}

type Report struct {
	simconnect.RecvSimobjectDataByType
	//Title    [256]byte `name:"TITLE"`
	Altitude            float64   `name:"AUTOPILOT ALTITUDE LOCK VAR" unit:"feet"`
	ComOneActiveFrequency  float64   `name:"COM ACTIVE FREQUENCY:1" unit:"Mhz"`
	ComOneStandbyFrequency float64   `name:"COM STANDBY FREQUENCY:1" unit:"Mhz"`
	ComTwoActiveFrequency  float64   `name:"COM ACTIVE FREQUENCY:2" unit:"Mhz"`
	ComTwoStandbyFrequency float64   `name:"COM STANDBY FREQUENCY:2" unit:"Mhz"`
	NavOneActiveFrequency  float64   `name:"NAV ACTIVE FREQUENCY:1" unit:"Mhz"`
	NavOneStandbyFrequency float64   `name:"NAV STANDBY FREQUENCY:1" unit:"Mhz"`
	NavTwoActiveFrequency  float64   `name:"NAV ACTIVE FREQUENCY:2" unit:"Mhz"`
	NavTwoStandbyFrequency float64   `name:"NAV STANDBY FREQUENCY:2" unit:"Mhz"`
	NavActiveFrequency  float64   `name:"COM ACTIVE FREQUENCY:2" unit:"Mhz"`
	ComStandbyFrequency float64   `name:"COM STANDBY FREQUENCY:2" unit:"Mhz"`
	Heading							float64   `name:"PLANE HEADING DEGREES MAGNETIC" unit:"degrees"`
	HeadingBug					float64   `name:"AUTOPILOT HEADING LOCK DIR" unit:"degrees"`
	HeadingMode					float64   `name:"AUTOPILOT HEADING LOCK"`
	ApproachMode				float64   `name:"AUTOPILOT APPROACH HOLD"`
	AltitudeMode				float64   `name:"AUTOPILOT ALTITUDE LOCK"`
	VerticalMode        float64   `name:"AUTOPILOT VERTICAL HOLD"`
	SpeedMode      		  float64   `name:"AUTOPILOT AIRSPEED HOLD"`
	FlcMode       		  float64   `name:"AUTOPILOT FLIGHT LEVEL CHANGE"`
	NavMode      				float64   `name:"AUTOPILOT NAV1 LOCK"`
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
