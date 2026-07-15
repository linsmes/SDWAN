//go:build !windows

package main

import (
	"net"

	"golang.zx2c4.com/wireguard/ipc"
)

func openUAPI(ifName string) (net.Listener, error) {
	fileUAPI, err := ipc.UAPIOpen(ifName)
	if err != nil {
		return nil, err
	}
	return ipc.UAPIListen(ifName, fileUAPI)
}
