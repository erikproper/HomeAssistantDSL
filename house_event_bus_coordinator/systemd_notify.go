/*
 *
 * Module:    HouseEventBusCoordinator
 * Package:   Main
 * Component: SystemdNotify
 *
 * PROJECT.md 19c (2026-10-27): the coordinator tells systemd it is ready once every subscription is
 * in place (sd_notify's "READY=1" on $NOTIFY_SOCKET; coordinator.service is Type=notify), so
 * "systemctl restart coordinator" -- deploy.d/00_deploy.coordinator -- returns only then. Found live
 * in the 19c pilot (deploys A and B): the restart returned at once, the next deploy step's reload
 * request arrived before the coordinator subscribed to it, and was lost.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 27.10.2026
 *
 */

package main

import (
	"fmt"
	"net"
	"os"
)

// notifySystemdReady sends "READY=1" to systemd; a no-op when not run by systemd as Type=notify.
func notifySystemdReady() error {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return nil
	}
	if socket[0] == '@' {
		socket = "\x00" + socket[1:] // an abstract socket
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socket, Net: "unixgram"})
	if err != nil {
		return fmt.Errorf("connecting to systemd's notify socket: %w", err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("READY=1")); err != nil {
		return fmt.Errorf("notifying systemd: %w", err)
	}
	return nil
}
