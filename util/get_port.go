package util

import (
	"net"
	"strconv"
)

func GetTCPPort() (string, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", err
	}
	if err = l.Close(); err != nil {
		return "", err
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return "", err
	}
	return port, nil
}

func GetTCPAndUDPPort() (string, error) {
	for {
		port, err := GetTCPPort()
		if err != nil {
			return "", err
		}
		portInt, _ := strconv.Atoi(port)
		l, err := net.ListenUDP("udp", &net.UDPAddr{Port: portInt})
		if err := l.Close(); err != nil {
			return "", err
		}
		if err == nil {
			return port, nil
		}
	}
}
