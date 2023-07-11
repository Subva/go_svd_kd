package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	//    "bytes"
	//"io/ioutil"
	"math"
	"strconv"
	"strings"

	//    "encoding/binary"
	//    "encoding/hex"
	"encoding/json"
	//@@ "./drv"     //"gitlab.mrgeng.ru/svd/drv"
	//@@ pb "./pbuf" //"gitlab.mrgeng.ru/svd/pbuf"
)

// ---------------------------------------
const (
	//DeviceDriverType = "ddm_ssoftrans" // Тип подглючаемх устройств, код ДДМ по умолчанию
	//  --
	DeviceDriverType = "ddm_ssoftv6" // Тип подглючаемх устройств, код ДДМ по умолчанию

	typeSSoftGM = "SSoftTrans"

	DeviceSignature_Length = 4 //размер представления идентификатора контроллера в сигнатуре

	Default_CorAddress = "?" //адрес корректора по умолчанию не известен
	Default_CorSpeed   = 0   //скорость обмена с корректором по уолчанию

	Default_ResponseTimeout    = 5 //тайм-аут ожидания ответа по умолчанию
	Default_SendAttempt        = 3 //количество повторов последней команды по умолчанию
	Default_OpenSessionAttempt = 5 //количество повторов сессии по умолчанию

	OpenSessionResponse_MinLength = 12 //минимальная длина ответа от корректоров типа ЕК
	OpenSessionResponse_MaxLength = 24 //максимальная длина ответа от корректоров типа ЕК
	//InterCharacter_Timeout        = 100  //межсимвольный тайм-аут в милисекундах
	MaxLenBuffer = 1024 //максимальная длина ответа корректора

	drv_ver = "0.52"
)

// ---ВЗАИМОДЕЙСТВИЕ С СВД-------------------------------------------------------
type CmdEK struct { // Команда для устройства modbus
	tms time.Time // время передачи команды
	cmd int       // код команды
}

// ------------------------------------------------------------------------------
type DrvData struct { // возвращает DeviceIdentification, сессионная межпоточная структура хранения
	UID    string // уникальный идентификатор устройства, возвращается при подключении
	Buffer []byte // требуется для некоторых драйверов
	// Постоянное сессионное хранилище, всё что понадобиться передать в сессию и далее на обработку
	SesInd uint16    // идентификатор пакета
	ShTime time.Time // время шедулера
	//CMB    CmdMB     // Команда для устройств modbus
	CEK CmdEK // Команда для устройств EKxx

	iddm map[string]string // индекс параметров DDM для этого устройства
	//@@ curRequests []drv.ReqMB       // Предварительно инициализированный пакет текущих запросов
	//    ird        int                      // количество прочитанных байт
	Drv             *DeviceMeterDriver
	CmdID           string    //идентификатор текущей выполняемой сессии
	BeginConnection time.Time //время подключения устройства
}

// @@// ---ВЗАИМОДЕЙСТВИЕ С СВД-------------------------------------------------------
// @@...
// ---ИТЕРФЕЙС ДРАЙВЕРА С КАНАЛОМ СВЯЗИ -----------------------------------------
type Connecter interface { //интерйес с каналом связи
	WriteToConn([]byte) (int, error)            //[]byte - передаваемый массив байт, int - длина переданых данных, error - ошибка передачи данных
	ReadFromConn(time.Duration) ([]byte, error) //time.Duration - тайм-аут оиждания данных, []byte - полученный массив байт, error - ошибка получения данных
	//ReadFromConn(time.Duration, []byte) (int, error) //time.Duration - тайм-аут оиждания данных, []byte - полученный массив байт, int - длина полученных данных, error - ошибка получения данных
}

// ---СТРУКТУРА ДЛЯ ВЫВОДА РЕЗУЛЬТАТА В БУФЕР ФОРМАТА JSON
//
//	{
//		"DeviceType":"EK270"
//		"DeviceAddress":""
//		"DeviceComSpeed":9600
//	}
type Settings struct {
	DevType           string `json:"DeviceType"`
	Address           string `json:"DeviceAddress"`
	Speed             int    `json:"DeviceComSpeed"`
	StateConsumerLock string `json:"StateConsumerLock"`
	StateVendorLock   string `json:"StateVendorLock"`
}

// ---РЕАЛИЗАЦИЯ В СОСТАВЕ УТИЛИТЫ АВТОПРЕДЕЛЕНИЯ НАСТРОЕК ВРГ--------------------
// типы подлежащие реализации в утилите для обеспечения сборки:
// @@	DrvData, Device, Metric, SVDScript
type Device struct {
	UID string
}

type Args struct {
	wake string        //будильник
	to   time.Duration //тайм-аут ответа на команду
}

func (dmd *DeviceMeterDriver) ProcessADS(sc Connecter, args Args) (outdata []byte, err error) {
	//log := log.New(os.Stdout, SetFlags(log.Ldate|log.Ltime))
	//dev := make([]Device, 1)
	dd := &DrvData{UID: "unknown"}
	err = dmd.Initialize(nil, dd)
	dmd.Cor.Timeout = args.to
	if err != nil {
		return nil, err
	}
	dd.BeginConnection = time.Now()
	dmd.CurStage = Awaken
	repeatSession := dmd.Cor.AS
	for {
		err = dmd.SendRequest(sc)
		if err != nil {
			break
		}
		if dmd.CurStage == Ending {
			break
		}
		//получение и обработка ответа
		err = dmd.ProcessResponse(sc) //здесь же вывод результата в ss.Devs[0].Data
		if err != nil {
			if strings.Index(err.Error(), "connection error") != -1 {
				//fmt.Println(err)
				break
			}
			if strings.Index(err.Error(), "skip the error") == -1 {
				repeatSession--
				if repeatSession <= 0 {
					repeatSession = 0
					break
				} else {
					dmd.CurStage = Awaken
				}
			}
		}
		if /*dmd.CurStage == LockStatus ||*/ dmd.CurStage == ReadWrite {
			dmd.CurStage = CloseInterface
		} else if dmd.CurStage == Ending {
			break
		}
	}

	//сохранение результата
	g := dmd.Cor.Groups[dmd.Cor.GrIndex["sys"]]
	var settings Settings
	//3-тип корректора
	if len(g.Params[3].Values) > 0 {
		settings.DevType = g.Params[3].Values[0].Value
	} else {
		settings.DevType = ""
	}
	//4-адрес корректора
	if len(g.Params[4].Values) > 0 && !g.Params[4].Values[0].Timestamp.IsZero() {
		settings.Address = g.Params[4].Values[0].Value
	} else {
		settings.Address = ""
	}
	//скорость обмена корректора
	vals := g.Params[5].Values
	if len(vals) > 0 && !vals[0].Timestamp.IsZero() {
		settings.Speed = func(i string) int {
			n, err := strconv.Atoi(i)
			if err == nil && !(n < 0 || n > 7) { //7-максимальная допустимая скорость = 38400
				return 300 * int(math.Pow(2, float64(n)))
			} else {
				return 0
			}
		}(vals[0].Value)
	} else {
		settings.Speed = 0
	}
	//1-состояние замка потребителя
	if len(g.Params[1].Values) > 0 {
		s := g.Params[1].Values[0].Value
		if s == "1" {
			settings.StateConsumerLock = "opened"
		} else {
			settings.StateConsumerLock = "closed"
		}
	} else {
		settings.StateConsumerLock = ""
	}
	//2-состояние замка поставщика
	if len(g.Params[2].Values) > 0 {
		s := g.Params[2].Values[0].Value
		if s == "1" {
			settings.StateVendorLock = "opened"
		} else {
			settings.StateVendorLock = "closed"
		}
	} else {
		settings.StateVendorLock = ""
	}

	outdata = nil
	if settings.DevType != "" || settings.Address != "" || settings.Speed != 0 || settings.StateConsumerLock != "" || settings.StateVendorLock != "" {
		var errm error
		outdata, errm = json.Marshal(settings)
		if errm != nil {
			err = errors.New(err.Error() + " | " + errm.Error())
		}
	}

	//очистка драйвера
	dmd.Cor.ClearAddressSpace(true)

	return outdata, err
}

// ---ДРАЙВЕР -------------------------------------------------------------------
// управление сценарием взаимодействия с корректором (уровень устройства)
// задает алгоритм взаимодействия с корректором посредством этапов взаимодействия
type Stage uint

const ( //выполняемые драйвером этапы взаимодействия
	Awaken         Stage = iota //формирование последовательности пробуждения ("будильника")
	OpenInterface               //открытие интерфейса
	SpeedAgreement              //согласование скорости обмена
	LockStatus                  //чтение состояния замка
	OpenLock                    //открытие замка
	ReadWrite                   //чтение или запись параметров|архивов|журналов
	CloseLock                   //закрытие замка
	CloseInterface              //закрытияе интерфейса
	Ending
)

type DeviceMeterDriver struct {
	UID string //уникальный идентификатор корректора
	Cor Corrector

	CurStage Stage //индекс текущего этапа
}

func (dmd *DeviceMeterDriver) Initialize(dev *Device, dd *DrvData /*, ds pb.SVDScript*/) error {
	//	if dd.Drv == nil {
	//		return fmt.Errorf("сессия с устройством %s не открыта", dd.UID)
	//	}
	log.Println("INFO: Initialize():", dd.UID, ", drv_ver.", drv_ver) //dd.UID != "" при наличии соединения с коннектором
	/*var i int
	for i, dev := range devs {
		//strmess := fmt.Sprintf("INFO: Initialize() Devs[%d]: %s", i, dev.UID) //--
		if dd.UID == dev.UID {
			//log.Println(strmess, "+") //--
			break
		} //else {
		//log.Println(strmess, "-") //--
		//}
	}
	if i >= len(devs) {
		return fmt.Errorf("устройство не обнаружено %s", dd.UID) // V - devs[1].UID - V
	}*/
	dmd.Cor.ClearAddressSpace(true)
	if dev != nil {
		dmd.UID = dev.UID
	} else {
		dmd.UID = dd.UID
	}
	dmd.Cor.GrIndex = make(map[string]int)
	dmd.CurStage = Ending
	dmd.Cor.RepeatCom = 0
	dmd.Cor.RepeatSes = dmd.Cor.RepeatCom
	//инициализация полей значениями по умолчанию
	dmd.Cor.Address = Default_CorAddress
	dmd.Cor.Speed = Default_CorSpeed
	dmd.Cor.Timeout = time.Second * Default_ResponseTimeout
	dmd.Cor.AR = Default_SendAttempt
	dmd.Cor.AS = Default_OpenSessionAttempt

	//???загрузка настроек устройства (адрес, номер канала, пароли и т.д.)
	//???... из ss.DC[0].Addr и ss.DC[0].Prms ?
	//добавление служебной группы параметров - используется в движке драйвера и может быть выдана как результат GETCUR
	gr, _ := dmd.Cor.AddGroup("sys", "GETCUR")
	gr.Params = append(gr.Params, Parameter{PID: "MS", Address: "MS", VType: "s"})         //0 - добавление внутреннего параметра (PID == Address) - признак открытия замка: ""-нет, "4:170" или "3:170"
	gr.Params = append(gr.Params, Parameter{PID: "4:170", Address: "4:170", VType: "d"})   //1 - состояние замка потребителя: 0-закрыт, 1-открыт
	gr.Params = append(gr.Params, Parameter{PID: "3:170", Address: "3:170", VType: "d"})   //2 - состояние замка поставщика: 0-закрыт, 1-открыт
	gr.Params = append(gr.Params, Parameter{PID: "DT", Address: "DeviceType", VType: "s"}) //3 - тип корректора
	gr.Params = append(gr.Params, Parameter{PID: "Addr", Address: "Address", VType: "s"})  //4 - адрес корректора: "" или "0"
	gr.Params = append(gr.Params, Parameter{PID: "DS", Address: "Speed", VType: "s"})      //5 - скорость обмена с корректором
	log.Println("INFO: DeviceMeterDriver.Initialize()-end +param=", len(gr.Params))

	return nil
}

// func (dmd *DeviceMeterDriver) CreateAddressSpace(ds pb.SVDScript) error {
//@@...
// func (dmd *DeviceMeterDriver) HoldConnect(sc Connecter, dev *Device, dd *DrvData) error {
//@@...
// func (dmd *DeviceMeterDriver) DeviceExchange(sc Connecter, dev *Device) error {
//@@...

func (dmd *DeviceMeterDriver) SendRequest(conn Connecter) (err error) {
	switch dmd.CurStage {
	case Awaken:
		err = dmd.Cor.WakwUpExchange(conn)
		fallthrough
	case OpenInterface:
		dmd.CurStage = OpenInterface
		_, err = dmd.Cor.OpenSessionRequest(conn)
	case SpeedAgreement:
		_, err = dmd.Cor.SetSpeedCoordinationRequest(conn)
	case LockStatus:
		_, err = dmd.Cor.ReadStateLockRequest(conn)
	case OpenLock:
		_, err = dmd.Cor.UnLockRequest(conn)
	case ReadWrite:
		switch dmd.Cor.Groups[dmd.Cor.Curgroup].Cmd {
		case "R1":
			_, err = dmd.Cor.ReadParamRequest(conn)
		case "R3":
			_, err = dmd.Cor.ReadArchiveRequest(conn)
		case "W1":
			_, err = dmd.Cor.WriteParamRequest(conn)
		default:
			err = fmt.Errorf("Неизвестная команда %q", dmd.Cor.Groups[dmd.Cor.Curgroup].Cmd)
		}
	case CloseLock:
		_, err = dmd.Cor.LockRequest(conn)
	case CloseInterface:
		_, err = dmd.Cor.CloseSessionRequest(conn)
		dmd.CurStage = Ending
	default:
		err = fmt.Errorf("unknown stage of interaction: %d", dmd.CurStage)
		return nil
	}
	//if err != nil { }
	return err
}

func (dmd *DeviceMeterDriver) ProcessResponse(conn Connecter) (err error) {
	var buf []byte
	next := 1
	var n int
	stage := dmd.CurStage
	if /*cd.CurStage == Awaken ||*/ dmd.CurStage == OpenInterface {
		//conn.SetReadDeadline(time.Now().Add(dmd.Cor.Timeout))
		//buf = make([]byte, 20)
		//n, err = conn.Read(buf)
		buf, err = conn.ReadFromConn(dmd.Cor.Timeout)
		if err != nil {
			return fmt.Errorf("connection error: %s", err.Error())
		} else {
			//if strings.Index(err.Error(), "i/o timeout") != -1 {
			//	log.Println(err)
			//	dmd.Cor.RepeatCom++
			//	if dmd.Cor.AR >= dmd.Cor.RepeatCom {
			//		next = 0
			//		err = nil
			//	}
			//} else {
			//	log.Println("->", err)
			//	next = 1
			//}
			//} else {
			log.Println("->(", len(buf), ")", String(buf), " | ", SprintBytes(buf))
			next, err = dmd.Cor.OpenSessionResponce(buf)
			if err == nil {
				dmd.Cor.RepeatCom = 0
			}
		}
	} else {
		var tout time.Duration = dmd.Cor.Timeout         //тайм-аут ожидания ответа time.Millisecond * InterCharacter_Timeout //межсимвольный интервал времени
		buf, err = /*drv.*/ MReadMsgIEC61107(conn, tout) // чтение по-байтное
		if err != nil {
			log.Println("->", err)
			if strings.Index(err.Error(), "connection error:") != -1 {
				return err
			}
		} else {
			//log.Println("->", String(buf), " | ", SprintBytes(buf))
			switch dmd.CurStage {
			case SpeedAgreement:
				next, err = dmd.Cor.SetSpeedCoordinationReponse(buf)
			case LockStatus:
				next, err = dmd.Cor.ReadStateLockResponse(buf)
				if next == 1 {
					dmd.CurStage = OpenLock //???пока не проверено открытие замка - пропускаем этап
				}
			case OpenLock:
				next, err = dmd.Cor.UnLockResponse(buf)
			case ReadWrite:
				if buf[0] == '7' && buf[1] == '-' {
					n = 2
				}
				switch dmd.Cor.Groups[dmd.Cor.Curgroup].Cmd {
				case "R1":
					next, err = dmd.Cor.ReadParamResponse(buf[n:])
				case "R3":
					next, err = dmd.Cor.ReadArchiveResponse(buf[n:])
				case "W1":
					next, err = dmd.Cor.WriteParamResponse(buf[n:])
				default:
					err = fmt.Errorf("неизвестная команда: %s", dmd.Cor.Groups[dmd.Cor.Curgroup].Cmd)
				}
				if next == 1 {
					dmd.CurStage = CloseLock //???пока не проверено открытие замка - пропускаем этап
				}
			case CloseLock:
				next, err = dmd.Cor.LockResponse(buf)
				if next == 1 {
					dmd.CurStage = CloseInterface //???пока не проверено открытие замка - пропускаем этап
				}
			case CloseInterface:
				dmd.CurStage = Ending
			default:
				err = fmt.Errorf("unknown stage of interaction: %d", dmd.CurStage)
			}
			if err == nil {
				dmd.Cor.RepeatCom = 0
			}
		}
	}
	if err != nil { //повтор команд при ошибках dmd.Cor.AR число раз
		dmd.Cor.RepeatCom++
		log.Println(dmd.Cor.RepeatCom, "AR=", dmd.Cor.AR)
		if dmd.Cor.RepeatCom <= dmd.Cor.AR {
			next = 0
			dmd.CurStage = stage
			err = fmt.Errorf("skip the error")
		} else { //число повторов истекло - ...
			dmd.Cor.RepeatCom = 0
		}
	} else {
		//if err == nil {
		//dmd.Cor.RepeatCom = 0
		if next == 1 {
			if dmd.CurStage != Ending {
				dmd.CurStage++
			}
		} else if next == -1 {
			dmd.CurStage = Awaken
		}
	}
	return err
}

// func (cd *DeviceMeterDriver) SaveData(dev *Device) error {
// @@...
// ------------------------------------------------------------------------------
// реализации действия обмена с корректором по протоколу
// ------------------------------------------------------------------------------
type Value struct {
	Value     string    //собственно значение
	Timestamp time.Time //метка времени формирования значения
}

type Parameter struct {
	PID      string  //идентификатор параметра
	Address  string  //адрес параметра
	VType    string  //тип значения
	Values   []Value //массив значений
	Recvalue string  //значение записывамое при записи параметра
}

func (p *Parameter) SaveOneValue(res []byte, vts time.Time) error {
	strerr := p.Address
	la := len(p.Address)
	if string(res[:la]) == p.Address {
		strvalue := string(res[la+3 : len(res)-1]) //3 - пропуск ".d("
		i := strings.IndexAny(strvalue, "*()")
		if i != -1 {
			strvalue = strvalue[:i]
		}
		if len(strvalue) > 0 {
			p.Values = append(p.Values, Value{strvalue, vts})
			strerr = ""
		}
	}
	if len(strerr) > 0 {
		strerr += " " + string(res)
		return fmt.Errorf(strerr)
	}
	return nil
}

type GroupParam struct { //группа параметров корректора
	GID string //идентификатор группы
	//Channel uint32      //номер канала
	Cmd    string      //код команды (чтение/запись)
	Params []Parameter //список параметров группы

	//STS      time.Time //метка времени старта выполнения действия
	//BArchTS      time.Time //метка времени начала вычиваемого архива/журнала
	//EArchTS      time.Time //метка времени окончания вычиваемого архива/журнала
	//CountRecRsp uint      //заданное количество записей в ответе при чтении архмва
}

/*func (gr *GroupParam) GetParamByAddress(addr string) (ok bool, pr *Parameter) {
	for _, param := range gr.Params {
		if param.Address == addr {
			return true, &param
		}
	}
	return false, nil
}*/

// ------------------------------------------------------------------------------
type Corrector struct {
	DeviceType string //тип устроства
	//сценарий будильника
	Address string //адрес устройства
	Speed   uint8  //скорость обмена
	//PassC      string                //пароль потребителя
	//PassS      string                //пароль поставщика
	Groups  []*GroupParam  //массив групп параметров
	GrIndex map[string]int //массив индексов групп параметров

	Curgroup  uint8
	Curparam  uint16
	Curvalue  uint32
	RepeatCom int //количество истекших попыток повтора последней команды
	RepeatSes int //количество истекших попыток повтора открытия сеанса обмена с корректором

	Timeout time.Duration //тайм-аут ожидания ответа от УУ

	//BTS      time.Time //метка времени начала не вычитанного архива/журнала
	//ETS      time.Time //метка времени окончания не вычитанного архива/журнала
	//CountRec uint      //количество записей в ответе при чтении архмва
	AR int //количество попыток повтора последней команды
	AS int //количество попыток повтора сеанса обмена с корректором
}

func (cor *Corrector) AddGroup(gid, cmd string) (group *GroupParam, ok bool) {
	GetCommand := func(cmd string) string { //замена на команды IEC61107
		cmd_ek2xx := make(map[string]string)
		cmd_ek2xx["GETCUR"] = "R1"
		cmd_ek2xx["GETHIS"] = "R3"
		cmd_ek2xx["SETPRM"] = "W1"
		return cmd_ek2xx[cmd]
	}
	//
	var command string
	if command = GetCommand(cmd); command == "" {
		return nil, false
	}
	//
	names := []string{"sys", "Current", "Passport", "Prog", "DayArch", "HourArch", "LogAlarm", "LogChange"}
	for i, gr := range names {
		if gr == gid {
			group = &GroupParam{GID: gid, Cmd: command}
			cor.Groups = append(cor.Groups, group)
			cor.GrIndex[gr] = i
			return group, true
		}
	}
	return nil, false
}

// func (cor *Corrector) SaveData(dev *Device) error {
// @@...

// чистка адресного пространства: группы, параметры
func (cor *Corrector) ClearAddressSpace(includeSys bool) {
	var k = 0
	if !includeSys {
		k = 1
	}
	//чистка адресного пространства (обратная CreateAddressSpace()), кроме удаления группы "sys"
	for i := len(cor.Groups) - 1; i >= k; i-- {
		//...удаление значений параметров
		cor.GrIndex[cor.Groups[i].GID] = 0
		cor.Groups[i].Params = cor.Groups[i].Params[:0]
	}
	cor.Groups = cor.Groups[:k]
}

func (cor *Corrector) WakwUpExchange(conn Connecter) (err error) {
	/*/вариант от разработчика
	msg := make([]byte, 80)
	log.Println("<-", String(msg), " | ", SprintBytes(msg))
	_, err = conn.WriteToConn(msg)
	if err != nil {
		return err
	}*/

	/*/вариант 2 по опыту использования
	msg := []byte {'0', '0','0','0','0','0','0','0'}
	_, err = conn.WriteToConn(msg)
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 100)
	msg = make([]byte, 253)
	_, err = conn.WriteToConn(msg)
	if err != nil {
		return err
	}*/

	/*/вариант 3 по опыту использования
	msg := []byte {'0', '0','0','0','0','0','0','0'}
	_, err = conn.WriteToConn(msg)
	if err != nil {
		return err
	}
	time.Sleep(time.Millisecond * 100)
	for i := 0; i < 35; i++ {
		_, err = conn.WriteToConn(msg[:1])
		if err != nil {
			return err
		}
	}*/

	//вариант 4 по опыту использования
	_, err = cor.CloseSessionRequest(conn)
	if err != nil {
		return err
	} /*
		msg := make([]byte, 40)
		_, err = conn.WriteToConn(msg)
		if err != nil {
			return err
		}
		time.Sleep(time.Millisecond * 100)
		msg = make([]byte, 40)
		_, err = conn.WriteToConn(msg)
		if err != nil {
			return err
		}*/

	time.Sleep(time.Millisecond * 1500)

	return nil
}
func (cor *Corrector) OpenSessionRequest(conn Connecter) (req []byte, err error) { //2F-3F-21-0D-0A
	var address string
	p := &cor.Groups[cor.GrIndex["sys"]].Params[4] //4-индекс параметра "Addr" в группе
	if !strings.Contains(cor.Address, Default_CorAddress) {
		address = cor.Address
		if len(p.Values) == 0 {
			p.Values = append(p.Values, Value{Value: address})
		}
	} else {
		if len(p.Values) == 0 {
			address = ""
			p.Values = append(p.Values, Value{Value: address})
		} else {
			address = p.Values[0].Value
			if p.Values[0].Timestamp.IsZero() {
				if cor.RepeatCom >= cor.AR {
					address = "0" //= NextAddress(address)
					p.Values[0].Value = address
				}
			}
		}
	}

	req = []byte{0x2f, 0x3F}
	for _, c := range address { //добавление адреса
		req = append(req, byte(c))
	}
	req1 := []byte{0x21, 0x0D, 0x0A}
	req = append(req, req1...)

	log.Println("<-", String(req), " | ", SprintBytes(req))
	_, err = conn.WriteToConn(req)
	return req, err
}
func (cor *Corrector) OpenSessionResponce(res []byte) (nextstage int, err error) {
	//2F-45-6C-73-36-45-4B-32-37-30-0D-0A
	var strerr string = "ошибочный ответ на запрос открытия интерфейса: "
	n := len(res)
	//	strres := string(res[1:])
	if OpenSessionResponse_MinLength <= n && n <= OpenSessionResponse_MaxLength {
		//if res[0] == '/' && (strings.Index(strres, "/Els") == -1 || strings.Index(strres, "/AGE") == -1) {
		if res[0] == '/' && (res[1] == 'E' && res[2] == 'l' && res[3] == 's' || res[1] == 'A' && res[2] == 'G' && res[3] == 'E') {
			strerr = ""
		}
	}
	//
	if len(strerr) != 0 {
		return 0, fmt.Errorf("%s (%d)%s", strerr, len(res), SprintBytes(res))
	}
	//сохранение результатов
	g := cor.Groups[cor.GrIndex["sys"]] //... в группе системных параметров:
	//p := &g.Params[5]                   //-[5]-скорость обмена с корректором
	//if len(p.Values) != 0 {
	//	if p.Values[0].Timestamp.IsZero() {
	//		p.Values[0].Timestamp = time.Now()
	//	}
	//} else {
	//	p.Values = append(p.Values, Value{string(res[4]), time.Now()})
	//}

	cor.DeviceType = string(res[5 : n-2])
	p := &g.Params[3] //-[3] тип корректора
	if len(p.Values) == 0 {
		p.Values = append(p.Values, Value{cor.DeviceType, time.Now()})
	}

	p = &g.Params[4] //-[4]адрес корректора
	if len(p.Values) != 0 && p.Values[0].Timestamp.IsZero() {
		p.Values[0].Timestamp = time.Now()
	}
	//
	return 1, nil
}

func (cor *Corrector) SetSpeedCoordinationRequest(conn Connecter) (req []byte, err error) {
	//06-30-36-31-0D-0A
	req = []byte{0x06, 0x30, 0x00, 0x31, 0x0D, 0x0A}
	var speed uint8
	p := &cor.Groups[cor.GrIndex["sys"]].Params[5] //5-индекс параметра "DS" в группе
	if cor.Speed != 0 {
		speed = cor.Speed
		if len(p.Values) == 0 {
			p.Values = append(p.Values, Value{Value: string(speed)})
		}
	} else {
		if len(p.Values) == 0 {
			speed = '5'
			p.Values = append(p.Values, Value{Value: string(speed)})
		} else {
			speed = p.Values[0].Value[0]
			if p.Values[0].Timestamp.IsZero() {
				if cor.RepeatCom >= cor.AR {
					speed, _ = func(sp uint8) (uint8, error) {
						if sp < 0x30 || sp > 0x37 {
							return 0, fmt.Errorf("недопустимое значение скорости обмена %X", sp)
						}
						res := sp + 1
						if res == 0x38 {
							res = 0x30
						}
						return res, nil
					}(speed)
					p.Values[0].Value = string(speed)
				}
			}
		}
	}

	req[2] = speed
	log.Println("<-", String(req), " | ", SprintBytes(req))
	_, err = conn.WriteToConn(req)
	return req, err
}
func (cor *Corrector) SetSpeedCoordinationReponse(res []byte) (nextstage int, err error) {
	//01-50-30-02-28-31-32-33-34-35-36-37-29-03-50
	var strerr string = "ошибочный ответ на запрос согласования скоростей: "
	var stretalon string = "P0 (1234567)"
	//
	if len(res) == len(stretalon) { //12 = len()
		res[2] = 0x20 //' '
		if string(res) == stretalon {
			p := &cor.Groups[cor.GrIndex["sys"]].Params[5]
			if len(p.Values) != 0 && p.Values[0].Timestamp.IsZero() {
				p.Values[0].Timestamp = time.Now()
			}
			strerr = ""
		}
	}
	if len(strerr) > 0 {
		return 0, fmt.Errorf(strerr + SprintBytes(res))
	}
	return 1, nil
}

func (cor *Corrector) ReadStateLockRequest(conn Connecter) (req []byte, err error) {
	//01-52-31-02-33-3A-31-37-30-2E-30-28-29-03-42
	//обработка только параметров "4:170" и "3:170" группы "sys"
	g := cor.Groups[cor.GrIndex["sys"]]
	p := g.Params[1]
	if len(p.Values) == 0 {
		req, err = MSendCmdIEC61107(conn, g.Cmd, p.Address+".0()")
		log.Println("<- ", g.Cmd, " ", p.Address, ".0() |", SprintBytes(req))
	} else {
		p = g.Params[2]
		if len(p.Values) == 0 {
			req, err = MSendCmdIEC61107(conn, g.Cmd, p.Address+".0()")
			log.Println("<- ", g.Cmd, " ", p.Address, ".0() |", SprintBytes(req))
		}
	}
	return req, err
}
func (cor *Corrector) ReadStateLockResponse(res []byte) (nextstage int, err error) {
	//02-33-3A-31-37-30-2E-30-28-31-29-03-12
	//  -33-3A-31-37-30-2E-30-28-31-29-
	var strerr string = "ошибочный ответ на чтение состояния замка "

	g := cor.Groups[cor.GrIndex["sys"]]
	p := &g.Params[1]
	if len(p.Values) > 0 { //значение параметра получено на предыдущих итерацих чтения
		p = &g.Params[2]
	}
	err = p.SaveOneValue(res, time.Now())
	if err != nil {
		strerr += err.Error()
	} else {
		strerr = ""
	}

	if p.PID == "4:170" { //1-индекс параметра Состояние замка поставщика //!!!после поменять на 2
		nextstage = 1
	} else {
		nextstage = 0
	}
	if len(strerr) != 0 {
		return 0, fmt.Errorf(strerr + SprintBytes(res))
	}
	return nextstage, nil
}

func (cor *Corrector) UnLockRequest(conn Connecter) (req []byte, err error) {

	//
	return nil, fmt.Errorf("stub")
}
func (cor *Corrector) UnLockResponse(res []byte) (nextstage int, err error) {
	//
	return 1, fmt.Errorf("stub")
}

func (cor *Corrector) ReadParamRequest(conn Connecter) (req []byte, err error) {
	//01-52-31-02-35-3A-32-31-30-5F-31-2E-30-28-30-29-03-1F
	if cor.Curgroup >= uint8(len(cor.Groups)) {
		err = fmt.Errorf("!!!выход за границы списка групп")
		return nil, err
	}
	g := cor.Groups[cor.Curgroup]
	if cor.Curparam >= uint16(len(g.Params)) { //выход за границы списка параметров группы
		err = fmt.Errorf("!!!выход за границы списка параметров группы")
		return nil, err
	}
	p := g.Params[cor.Curparam]
	if len(p.Values) == 0 {
		//попытка получить значение из внутреннего источника
		//!!временное решение
		if p.PID == "DeviceType" {
			vts := time.Now()
			p.Values = append(p.Values, Value{cor.DeviceType, vts})
			cor.Curparam++
			return nil, fmt.Errorf("...") //err=... - пропустить обработку ответа
		}
		//
		req, err = MSendCmdIEC61107(conn, g.Cmd, p.Address+".0(0)")
		log.Println("<- ", g.Cmd, " ", p.Address, ".0() |", SprintBytes(req))
	}
	return nil, err
}
func (cor *Corrector) ReadParamResponse(res []byte) (nextstage int, err error) {
	//02-35-3A-32-31-30-5F-31-2E-31-28-32-2E-39-39-2A-7B-43-29-03-41
	var strerr string = "ошибочный ответ на чтение параметра "
	vts := time.Now()

	//log.Println("ReadParamResponse(", len(res), ")", cor.Curgroup, cor.Curparam) //--
	p := &cor.Groups[cor.Curgroup].Params[cor.Curparam]
	if p != nil {
		if len(p.Values) > 0 { //значение параметра получено на предыдущих итерацих чтения
			log.Println("ReadParamResponse(): значение параметра получено на предыдущих итерацих чтения", p.Address, p.Values) //--
			cor.Curparam++
		} else {
			//log.Println("ReadParamResponse(): сохранение значения параметра") //--
			err = p.SaveOneValue(res, vts)
			if err != nil {
				strerr += err.Error()
			} else {
				strerr = ""
			}
		}
	}

	cor.Curparam++
	if cor.Curparam >= uint16(len(cor.Groups[cor.Curgroup].Params)) { //выход за границы списка параметров группы
		cor.Curgroup++
		if cor.Curgroup < uint8(len(cor.Groups)) {
			cor.Curparam = 0
		} else {
			nextstage = 1
		}
	} else {
		nextstage = 0
	}
	if len(strerr) != 0 {
		return 0, fmt.Errorf(strerr + SprintBytes(res))
	}
	//log.Println("ReadParamResponse(): ответ обработан") //--
	return nextstage, nil
}

func (cor *Corrector) ReadArchiveRequest(conn Connecter) (req []byte, err error) {
	return nil, fmt.Errorf("stub")
}
func (cor *Corrector) ReadArchiveResponse(res []byte) (nextstage int, err error) {
	return 1, fmt.Errorf("stub")
}

func (cor *Corrector) WriteParamRequest(conn Connecter) (req []byte, err error) {
	return nil, fmt.Errorf("stub")
}
func (cor *Corrector) WriteParamResponse(res []byte) (nextstage int, err error) {
	return 1, fmt.Errorf("stub")
}

func (cor *Corrector) LockRequest(conn Connecter) (req []byte, err error) {
	//
	return nil, fmt.Errorf("stub")
}
func (cor *Corrector) LockResponse(res []byte) (nextstage int, err error) {
	//
	return 1, fmt.Errorf("stub")
}

func (cor *Corrector) CloseSessionRequest(conn Connecter) (req []byte, err error) {
	//01-42-30-03-71
	msg := []byte{0x1, 0x42, 0x30, 0x03, 0x71}
	log.Println("<-", String(msg), " | ", SprintBytes(msg))
	_, err = conn.WriteToConn(msg)
	return req, err
}

// работа с параметрами корректора (уровеь параметра и его значени-я/й)
func String(arr []byte) (str string) { //представляет массив байт в виде строчных символов, где не печатываемые символы заменены на символ '.'
	for _, b := range arr {
		if b < 0x20 {
			b = '.'
		}
		str += fmt.Sprintf("%c", b)
	}
	if len(str) > 1 {
		return str[:len(str)-1]
	}
	return ""
}

func SprintBytes(arr []byte) (str string) { //представляет массив байт в строке, формата <код байта в 16-м представлении> с разделителем '-'
	for _, b := range arr {
		if b < 0x10 {
			str += fmt.Sprintf("0%X-", b)
		} else {
			str += fmt.Sprintf("%X-", b)
		}
	}
	if len(str) > 1 {
		return str[:len(str)-1]
	}
	return ""
}

func BufToData(buf string) []byte { //преобразует строковое представление массива байт в массив байт, где строка - это набор <байт в 16-й кодировке> с разделителем '-'
	var res []byte
	for _, c := range strings.Split(buf, "-") {
		if len(c) > 0 {
			b, _ := strconv.ParseUint(c, 16, 8)
			res = append(res, byte(b))
		} else {
			break
		}
	}
	return res
}

// МЭК 61107-2011 констатны
const (
	IEC61107_SOH = byte(0x01) // начало заголовка
	IEC61107_STX = byte(0x02) // начало текста
	IEC61107_ETX = byte(0x03) // конец текста
	IEC61107_EOT = byte(0x04) // конец блока
	IEC61107_ACK = byte(0x06) // подтверждение
	IEC61107_NAK = byte(0x15) // отрицательное подтвержение
	CR           = byte(0x0d)
	LF           = byte(0x0a)
)

// МЭК 61107-2011 Передача команды
func MSendCmdIEC61107(conn Connecter, scmd, param string) (data []byte, err error) {
	if len(scmd) != 2 || param == "" || len(param) > 32 {
		return nil, fmt.Errorf("command data failed")
	}
	//  --
	data = []byte{IEC61107_SOH, scmd[0], scmd[1], IEC61107_STX}
	data = append(data, []byte(param)...)
	data = append(data, IEC61107_ETX)             //ETX - конец текста
	data = append(data, MchekSumBccISO1155(data)) // BCC
	//  --
	//    fmt.Printf(">>:%X\n", data)
	_, err = conn.WriteToConn(data)
	//  --
	return data, err
} // МЭК 61107-2011 Чтение ответа
func MReadMsgIEC61107(conn Connecter, timeout time.Duration) (buf []byte, err error) { // чтение по байтное
	//conn.SetReadDeadline(time.Now().Add(to)) // таймаут одинания ответа
	//buf = make([]byte, MaxLenBuffer)
	//n, err := conn.Read(buf)
	buf, err = conn.ReadFromConn(timeout)
	n := len(buf)
	//n := LenClosePac(buf)
	//log.Printf("-->%d(%d): %s", k, n, SprintBytes(buf[:n])) //--
	if err != nil {
		return nil, fmt.Errorf("connection error: %s", err.Error())
	}
	if !(n > 0) {
		return nil, fmt.Errorf("no data: (%d)%s", n, SprintBytes(buf))
	}
	log.Println("->(", n, ")", String(buf), " | ", SprintBytes(buf))
	fl := byte(0)
	if fl == 0 {
		if buf[0] != IEC61107_STX && buf[0] != IEC61107_SOH {
			return nil, fmt.Errorf("first byte is not STX") //!!!break
		} // не начало
		fl = 1
	}
	if fl == 1 && (buf[0] == IEC61107_ETX || buf[0] == IEC61107_EOT) {
		fl = 2
	} // конец текста/блока
	//!!!}
	// проверить КС
	//!!!if len(buf) > 1 && chekSumBccISO1155(buf[:len(buf)-1]) == ch[0] {
	//!!!	return buf[1 : len(buf)-2], nil
	if n > 1 && MchekSumBccISO1155(buf[:n-1]) == buf[n-1] {
		return buf[1 : n-2], nil //возвращаются только данные бещ SOH ETX и BCC
	}
	return nil, fmt.Errorf("BCC read failed: (%d)%s", n, SprintBytes(buf))
}
func MchekSumBccISO1155(b []byte) (ks byte) {
	if b[0] == 0x01 || b[0] == 0x02 { // SOH or STX
		for _, h := range b[1:] {
			ks = ks ^ h
		}
	}
	return ks
}
func LenClosePac(buf []byte) int {
	var i int
	if len(buf) == 0 {
		return 0
	}
	n := len(buf) / 2
	if buf[n] != 0 {
		n = len(buf)
	}
	for i = n - 1; i >= 0 && buf[i] == 0; i-- {
	}
	return i + 1
}
