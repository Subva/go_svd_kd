package main

import (
	"container/list"
	"fmt"
	"log"
	"strings"
	"time"
)

// ------------------------------------------------------------------------------
// Драйвер устройства
// Универсальные для любого драйвера структуры и методы - "движок" драйвера
// ------------------------------------------------------------------------------
const (
	SKIP_WARNING = "..."
)

// ---ИТЕРФЕЙС ДРАЙВЕРА С КАНАЛОМ СВЯЗИ -----------------------------------------
type Connecter interface { //интерфейс с каналом связи
	WriteToConn([]byte) (int, error)                 //[]byte - передаваемый массив байт, int - длина переданых данных, error - ошибка передачи данных
	ReadFromConn([]byte, time.Duration) (int, error) //[]byte - получаемый массив байт, time.Duration - тайм-аут оиждания данных, длина полученных данных, error - ошибка получения данных
	//ReadFromConn(time.Duration, []byte) (int, error) //time.Duration - тайм-аут оиждания данных, []byte - полученный массив байт, int - длина полученных данных, error - ошибка получения данных
}

// ---ДРАЙВЕР -------------------------------------------------------------------
// управление сценарием взаимодействия с ecnhjqcndjv (уровень устройства)
// задает универсальный алгоритм взаимодействия с устройствам посредством этапов и шагов взаимодействия
type DeviceMeterDriver struct {
	cor    FlowComputer //данные
	stages []Stage      //вычислительный алгоритм, состоящий из стадий (для каждого дайвера индивидуален)
	cstage int          //текущая стадия взаимодействия с устройством от Start до Ending

	repeats int //количество истекших попыток повтора открытия сеанса обмена с корректором
}

func (dmd *DeviceMeterDriver) LoadConfigurationFor(groupnames []string) error {
	//загрузка конфигурации драйвера
	return fmt.Errorf("not implemented")
}

func (dmd *DeviceMeterDriver) DeviceExchange(sc Connecter) error {
	var err error
	var res ProcessResult
	if dmd.cstage == Finish {
		dmd.cstage = Start
	}
	dmd.repeats = dmd.cor.AS
	for {
		if dmd.cstage == Start {
			dmd.cstage++
		}
		curstg := dmd.stages[dmd.cstage]
		if curstg.closed {
			dmd.cstage++
			continue
		}
		res, err = curstg.SendRequest(sc)
		if err != nil {
			break
		} else {
			switch res.processing {
			case Begin:
				dmd.cstage = Start
				continue
			case Next:
				if curstg.closed {
					dmd.cstage++
				}
				continue
			case End:
				dmd.cstage = Finish
				//?continue
			}
		}
		if dmd.cstage == Finish {
			break
		}
		if curstg.pp.Len() == 0 {
			//--dmd.cstage++
			continue
		}
		res, err = curstg.ProcessResponse(sc)
		if err != nil {
			if strings.Index(res.msg, SKIP_WARNING) == -1 {
				dmd.repeats--
				if dmd.repeats <= 0 { //если истекло число повторов сессий
					dmd.cstage = Finish //завершить с ошибкой
				} else {
					dmd.cstage = Start //продолжить с начала
				}
			} else {
				dmd.cstage = Finish //завершить с ошибкой
			}
		} else {
			switch res.processing {
			case Begin:
				dmd.cstage = Start
			case Next:
				if curstg.closed {
					dmd.cstage++
				}
			case Last:
				dmd.cstage = LastStage
			case End:
				dmd.cstage = Finish
			}
		}
		if dmd.cstage == Finish {
			break
		}
	} //for
	if err != nil {
		log.Println("WARNING: %s", err.Error())
	}
	log.Println("INFO: завершение сеанса обмена с корректором")
	dmd.cstage = Finish
	//dmd.cor
	return fmt.Errorf("not implemented")
}

// ------------------------------------------------------------------------------
type Stage struct { //стадия обмена с устройством
	index int    //индекс этапа в списке этапов
	name  string //краткое описание этапа, назначение этапа
	lpp   []Step //список обрабатываемых параметров - объектов типа Step
	//curgroup uint
	//curparam uint
	//Curvalue  uint
	cor            *FlowComputer //представление пространства параметров устройства
	repeats        int           //количество истекших попыток повтора последней команды текущего шага
	closed         bool          //флаг завершения выполнения этапа
	bufferRequest  []byte        //отправляемый устройству пакет данных
	bufferResponse []byte        //принимаемый от устройства пакет данных
	pp             list.List     //список шагов, обрабатываемых в текущей команде устройству
}

func (stg *Stage) SendRequest(conn Connecter) (pr ProcessResult, err error) {
	//if stg.closed {
	//	return ProccesResult{processing: Next}, nil
	//}
	var n, k int
	var stp Step
	c := true //признак щакрытия стадии
	for i := 0; i < len(stg.lpp); {
		stp = stg.lpp[i]
		if !stp.closed {
			n, pr, err = stp.fQuery(stg.bufferRequest)
			if pr.kg {
				stg.pp.PushBack(&stp)
			}
			if err == nil {
				switch pr.processing {
				case Next, Prev:
					i += pr.processing
				case Begin, End:
					break
				default:
					i++
				}
			} else {
				break
			}
		} else {
			i++
		}
		c = c && stp.closed
	} //for
	if n != 0 {
		log.Println("<-", String(stg.bufferRequest[:n]), " | ", SprintBytes(stg.bufferRequest[:n]))
		k, err = conn.WriteToConn(stg.bufferRequest[:n])
		if k != n {
			err = fmt.Errorf("передано %d байт из %d", k, n)
		}
	}
	if c {
		stg.closed = true
		if pr.processing != Begin && pr.processing != End {
			pr.processing = Next
		}
	}
	return pr, err
}
func (stg *Stage) ProcessResponse(conn Connecter) (pr ProcessResult, err error) {
	//f stg.closed {
	//	return ProccesResult{processing: Next}, nil
	//}
	var stp *Step
	var el *list.Element
	var k int
	if stg.pp.Len() > 0 {
		el = stg.pp.Front()
		stp = el.Value.(*Step)
		k, err = conn.ReadFromConn(stg.bufferResponse, stp.timeout)
		if err != nil {
			log.Println("->", err)
			return ProcessResult{}, fmt.Errorf("connection error: %s", err.Error())
			//или вместо return следующее:
			//pr = ProccesResult{}
			//err = fmt.Errorf("connection error: %s", err.Error()
			//break //к очистке списка шагов в текущем ответе
		} else {
			log.Println("->(", k, ")", String(stg.bufferResponse[:k]), " | ", SprintBytes(stg.bufferResponse[:k]))
		}
	}
	for i := 0; i < stg.pp.Len(); {
		stp = el.Value.(*Step)
		if !stp.closed {
			_, pr, err = stp.fAnswer(stg.bufferResponse[:k], stg.bufferRequest)
			//? if err == nil swticth pr.processing
		}
		//fmt.Printf("%d-%s\n", stp.v, stp.parameter)
		i++
		el = el.Next()
	} //for
	stg.pp.Init()
	if err != nil {
		stg.repeats++
		if stg.repeats <= stg.cor.AR {
			//pr.processing = Same
			err = fmt.Errorf("skip the error")
		} else {
			stg.repeats = 0
		}
	} else {
		//? влияние pr.processing на обработку параметров из
	}
	c := true //признак щакрытия стадии
	for _, stpt := range stg.lpp {
		c = c && stpt.closed
	}
	if c {
		stg.closed = true
		if pr.processing != Begin && pr.processing != End {
			pr.processing = Next
		}
	}
	//...
	return pr, err
}

// ------------------------------------------------------------------------------
type Step struct {
	param      *Parameter    //ссылка на обрабатываемый параметр
	timeout    time.Duration //тайм-аут ожидания ответа от устройства на команду
	fBuild     Build         //метод преобразования входного значения
	fQuery     Send          //метод формирования запроса
	fAnswer    Receive       //метод обработки ответа
	fExport    Export        //метод преобразования значения-результата
	argsBuild  []*Argument   //аргументы метода преобразования входного значения
	argsQuery  []*Argument   //аргументы метода формирования запроса
	argsAnswer []*Argument   //аргументы метода обработки ответа
	argsExport []*Argument   //аргументы метода преобразования значения-результата
	closed     bool          //флаг завершения выполнения шага
}
type Build func(string) error
type Send func([]byte) (int, ProcessResult, error)
type Receive func([]byte, []byte) (int, ProcessResult, error)
type Export func() (string, error)
type Argument struct { //аргумент метода Build, Send, Receive или Export - любого назначения,
	//например, смещение значения параметра в буфере ответа устройства
	value string
	vtype string
}

// -------------------------------------------------------------------------------
const (
	Begin int = -100 //в начало
	Prev  int = -1   //предыдущий шаг или этап, если шаг первый в текущем этапе
	Same  int = 0    //тот же
	Next  int = 1    //следующий шаг или этап, если шаг последний в текущем этапе
	Last  int = 90   //последний этап
	End   int = 100  //закончить
)

type ProcessResult struct { //результат обработки шага/параметра
	kg         bool   //keep going - флаг необходимости дальнейшей обработки
	processing int    //(значения = [Begin, ... End])
	msg        string //служебные сообщения о результате обработки
}
