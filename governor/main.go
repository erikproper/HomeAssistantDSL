/*
 *
 * Module:    Governor
 * Package:   Main
 * Component: Main
 *
 * "governor <dir>": the governed integration's application (PROJECT.md 37,
 * claude-notes/plans/governed-integration.md). It reads <dir>/devices.yaml and <dir>/secrets.yaml
 * (written by the generator), connects to the main broker with "online"/"offline" on its status
 * topic (retained; "offline" as its last will), reads its desired states back for a few seconds,
 * then governs (governor.go). Every message is handled on one goroutine, with the clock ticking
 * the sending. It runs next to the coordinator, on the same machine, as a service of its own: the
 * one can be down without the other noticing.
 *
 * Creator: Henderik A. Proper (e.proper@acm.org), Junglinster, Luxembourg, in collaboration with Claude.ai
 *
 * Version of: 08.11.2026
 *
 */

package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	statusOnline  = "online"
	statusOffline = "offline"
	readBackTime  = 3 * time.Second // how long the retained desired states are read back at the start
	tickInterval  = 200 * time.Millisecond
)

// tMessage is a message for the governor's goroutine.
type tMessage struct {
	topic, payload string
	retained       bool
}

// tMQTTPublisher publishes through a paho client.
type tMQTTPublisher struct{ client mqtt.Client }

// Publish publishes payload on topic, at least once.
func (p tMQTTPublisher) Publish(topic, payload string, retained bool) {
	p.client.Publish(topic, 1, retained, payload)
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: governor <dir with devices.yaml and secrets.yaml>")
		os.Exit(1)
	}
	dir := os.Args[1]
	var devices TDevicesFile
	var secrets TSecretsFile
	if err := loadYAML(filepath.Join(dir, "devices.yaml"), &devices); err != nil {
		logger.Fatalf("error: %v", err)
	}
	if err := loadYAML(filepath.Join(dir, "secrets.yaml"), &secrets); err != nil {
		logger.Fatalf("error: %v", err)
	}
	if problems := checkDevicesFile(devices); len(problems) > 0 {
		for _, problem := range problems {
			logger.Printf("error: %s", problem)
		}
		os.Exit(1)
	}
	logger.Printf("governing %d entities", len(devices.Entities))

	messages := make(chan tMessage, 1000)
	handler := func(_ mqtt.Client, m mqtt.Message) {
		messages <- tMessage{topic: m.Topic(), payload: string(m.Payload()), retained: m.Retained()}
	}
	var g *TGovernor
	var started atomic.Bool
	subscribe := func(c mqtt.Client, topics []string) {
		for _, topic := range topics {
			c.Subscribe(topic, 1, handler)
		}
	}

	opts := mqtt.NewClientOptions()
	scheme := "tcp"
	if secrets.MQTT.TLS {
		scheme = "ssl"
		opts.SetTLSConfig(&tls.Config{})
	}
	opts.AddBroker(fmt.Sprintf("%s://%s:%s", scheme, secrets.MQTT.Server, secrets.MQTT.Port))
	hostname, _ := os.Hostname()
	opts.SetClientID("governor_" + hostname)
	opts.SetUsername(secrets.MQTT.Login)
	opts.SetPassword(secrets.MQTT.Password)
	opts.SetAutoReconnect(true)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetWill(devices.Status, statusOffline, 1, true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		c.Publish(devices.Status, 1, true, statusOnline)
		if started.Load() {
			subscribe(c, g.CommandTopics())
			subscribe(c, g.ReceivedTopics())
		}
		logger.Printf("connected to %s:%s", secrets.MQTT.Server, secrets.MQTT.Port)
	})
	client := mqtt.NewClient(opts)
	g = newGovernor(devices, tMQTTPublisher{client}, logger.Printf)
	if token := client.Connect(); !token.WaitTimeout(30*time.Second) || token.Error() != nil {
		logger.Fatalf("error: cannot connect to %s:%s: %v", secrets.MQTT.Server, secrets.MQTT.Port, token.Error())
	}

	// The desired states the broker kept.
	subscribe(client, g.StateTopics())
	readBack := time.After(readBackTime)
reading:
	for {
		select {
		case m := <-messages:
			g.RetainedState(m.topic, m.payload)
		case <-readBack:
			break reading
		}
	}
	for _, topic := range g.StateTopics() {
		client.Unsubscribe(topic)
	}
	g.Start(time.Now())
	started.Store(true)
	subscribe(client, g.CommandTopics())
	subscribe(client, g.ReceivedTopics())
	logger.Printf("governing")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(tickInterval)
	for {
		select {
		case m := <-messages:
			if m.retained {
				continue // a state from before; only live messages count now
			}
			now := time.Now()
			g.Command(m.topic, m.payload, now)
			g.Received(m.topic, m.payload, now)
		case now := <-ticker.C:
			g.Tick(now)
		case <-stop:
			client.Publish(devices.Status, 1, true, statusOffline).WaitTimeout(2 * time.Second)
			client.Disconnect(500)
			return
		}
	}
}
