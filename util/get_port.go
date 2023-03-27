package util

import (
	"golang.org/x/exp/slices"
	"net"
	"strconv"
	"sync"
	"time"
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
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if err := l.Close(); err != nil {
			return "", err
		}
		return port, nil
	}
}

type PortPool struct {
	mu            *sync.Mutex
	reservedPorts []string
}

func NewPortPool() *PortPool {
	return &PortPool{mu: new(sync.Mutex)}
}

func (p *PortPool) GetAndReserve() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var port string
	var err error
	for {
		port, err = GetTCPAndUDPPort()
		if err != nil {
			return "", err
		}
		if !slices.Contains(p.reservedPorts, port) {
			break
		}
	}
	p.reservedPorts = append(p.reservedPorts, port)
	return port, nil
}

func (p *PortPool) Release(port string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := slices.Index(p.reservedPorts, port)
	if idx == -1 {
		return
	}
	p.reservedPorts = slices.Delete(p.reservedPorts, idx, idx+1)
}
