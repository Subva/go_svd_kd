package main

import (
	"fmt"

	//    "bytes"
	//"io/ioutil"

	"strconv"
	"strings"
	//    "encoding/binary"
	//    "encoding/hex"
	//"gitlab.mrgeng.ru/Training/test_drv_ek2xx_iec/drvtest/ss_ek2XX/drv"
	//"./drv"
	//"./pbuf"
)

// представляет массив байт в виде строчных символов, где не печатываемые символы заменены на символ '.'
func String(arr []byte) (str string) {
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

// представляет массив байт в строковом виде, формата <код байта в 16-м представлении> с разделителем '-'
func SprintBytes(arr []byte) (str string) {
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

// преобразует строковое представление массива байт в массив байт, где строка - это набор <байт в 16-й кодировке> с разделителем '-'
func BufToData(buf string) []byte {
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
