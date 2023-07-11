package main

import (
	"net"
	"time"
)

const (
	DeviceDriverType = "ddm_..."  // тип подглючаемх устройств, код ДДМ по умолчанию
	typeSSoftGM      = "...Trans" // префикс кода свд (Код устройства в конфигурации устройств)

	DeviceSignature_Length = 4 //размер представления идентификатора контроллера в сигнатуре
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

	iddm        map[string]string // индекс параметров DDM для этого устройства
	curRequests []drv.ReqMB       // Предварительно инициализированный пакет текущих запросов
	//    ird        int                      // количество прочитанных байт
	Drv             *DeviceMeterDriver
	CmdID           string    //идентификатор текущей выполняемой сессии
	BeginConnection time.Time //время подключения устройства
}

//	-------------------------------------------------------------------------
//	-- Сервер внешних подключений. Контур взаимодействия с устройством  -----
//	-------------------------------------------------------------------------
//
// идентифицировать подключившегося к сокету
func DeviceIdentification(conn net.Conn) DrvData {
	//...
	return DrvData{}
}

// Процесс взаимодействия с внешним подключением  ---------------------------
func (ss *Session) RunServerInteractiveProcess(dd *DrvData) { // устройство подключилось к сокету,опознано и сконфигурировано.
	//...
} // завершить подключение

// -- Контроль и Управление. Контур взаимодейчтвия с APP.  -----------------
// -- Формирование заданий для исполнения в RunInteractiveProcess  ---------
func (ss *Session) RunInteractionControl(ds pb.SVDScript) (err error) {
	//...
	return nil
}

// Методы для взаимодействия драйвера с каналом связи СВД
func (ss *Session) WriteToConn(req []byte) (int, error) {
	return ss.Connect.Write(req)
}
func (ss *Session) ReadFromConn(res []byte, to time.Duration) (int, error) {
	ss.Connect.SetReadDeadline(time.Now().Add(to))
	n, err := ss.Connect.Read(res)
	if n == 0 || err != nil {
		return 0, err
	} else {
		return n, err
	}
}
