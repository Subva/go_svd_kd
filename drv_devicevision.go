package main

import (
	//"container/list"
	//"fmt"
	//"log"
	"fmt"
	"strings"
	"time"
)

// ------------------------------------------------------------------------------
// Драйвер устройства
// Представление адресного пространcтва устройства памяти драйвера
// ------------------------------------------------------------------------------
type FlowComputer struct { //Corrector
	DeviceType string //тип устроства
	//сценарий будильника
	Address string //адрес устройства
	Speed   uint8  //скорость обмена
	//PassC      string                //пароль потребителя
	//PassS      string                //пароль поставщика
	Groups  []*GroupParam  //массив групп параметров
	GrIndex map[string]int //массив индексов групп параметров

	Timeout time.Duration //тайм-аут ожидания ответа от УУ

	//BTS      time.Time //метка времени начала не вычитанного архива/журнала
	//ETS      time.Time //метка времени окончания не вычитанного архива/журнала
	//CountRec uint      //количество записей в ответе при чтении архмва
	AR int //количество попыток повтора последней команды
	AS int //количество попыток повтора сеанса обмена с корректором
}

func (cor *FlowComputer) AddGroup(gid, cmd string) (group *GroupParam, ok bool) {
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
	for i, gr := range groupname {
		if gr == gid {
			group = &GroupParam{GID: gid, Cmd: command}
			cor.Groups = append(cor.Groups, group)
			cor.GrIndex[gr] = i
			return group, true
		}
	}
	return nil, false
}

// чистка адресного пространства: группы, параметры
func (cor *FlowComputer) ClearAddressSpace(includeSys bool) {
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

var groupname = []string{"sys", "Current", "Passport", "Prog", "DayArch", "HourArch", "LogAlarm", "LogChange"}

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

// описание параметра устройства
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

// представление значения параметра устройства
type Value struct {
	Value     string    //собственно значение
	Timestamp time.Time //метка времени формирования значения
}
