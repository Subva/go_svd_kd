package main

import (
	"fmt"
)

// -----------------------------------------------------------------------------
// Драйвер Вымпел-500
// Описание драйвера заданного устройства и уникальные стуктуры и алгоритмы
// ------------------------------------------------------------------------------
const (
	devicename      = "Комплекс измерительный ультразвуковой \"Вымпел-500\""
	devicesignature = "vimpel500_mbrtu" //обозначение устройста и поддерживаемый протокол
	drv_ver         = "0.0.0.1"
)

// идентифификация этапов ------------------------------------------------------
const ( //перечень и количество этапов определяется при разработе драйвера
	Start          = iota           //начало выполнения первого этапа (обязательный)
	OpenInterface                   //открытие интерфейса
	ReadWrite                       //чтение или запись параметров|архивов|журналов
	CloseInterface                  //закрытия интерфейса
	LastStage      = CloseInterface //последний этап(обязательный)
	Finish                          //завершение выполнения последнего этапа (обязательный)
)

// ---ДРАЙВЕР -------------------------------------------------------------------
func (dmd *DeviceMeterDriver) Initialize() error {
	//создание группы системных параметров и инициализация движка драйвера
	return fmt.Errorf("not implemented")
}
func (dmd *DeviceMeterDriver) CreateAddressSpace( /*ds /*pb./ SVDScript*/ ) error {
	//создание требуемых групп параметров и загрузка конфигурации движка для чтени/записи данных групп
	return fmt.Errorf("not implemented")
}

// Методы шагов движка
// type Build func(string) error
// type Send func([]byte) (int, ProcessResult, error)
// type Receive func([]byte, []byte) (int, ProcessResult, error)
// type Export func() (string, error)
// type Argument struct { //аргумент метода Build, Send, Receive или Export
//	value string
//	vtype string
//}
