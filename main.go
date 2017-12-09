package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var temperature = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "thermometer_temperature_celsius",
		Help: "Current temperature of the thermometer.",
	},
	[]string{"thermometer"},
)

var humidity = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "hygrometer_humidity_percent",
		Help: "Current humidity of the hygrometer.",
	},
	[]string{"hygrometer"},
)

func init() {
	prometheus.MustRegister(temperature)
	prometheus.MustRegister(humidity)
}

func onSensor(client MQTT.Client, message MQTT.Message) {
	// hygrometers/sovrum/name
	// hygrometers/sovrum/value
	// thermometers/sovrum/name
	// thermometers/sovrum/value
	parts := strings.SplitN(message.Topic(), "/", 3)
	if parts[2] == "value" {
		name := parts[1]
		value, err := strconv.ParseFloat(string(message.Payload()), 64)
		if err == nil {
			if parts[0] == "thermometers" {
				temperature.WithLabelValues(name).Set(value)
			} else if parts[0] == "hygrometers" {
				humidity.WithLabelValues(name).Set(value)
			}
		}
	}
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("signal received, exiting")
		os.Exit(0)
	}()

	hostname, _ := os.Hostname()

	server := flag.String("server", "tcp://127.0.0.1:1883", "The full url of the MQTT server to connect to ex: tcp://127.0.0.1:1883")
	clientid := flag.String("clientid", hostname+strconv.Itoa(time.Now().Second()), "A clientid for the connection")
	username := flag.String("username", "", "A username to authenticate to the MQTT server")
	password := flag.String("password", "", "Password to match username")
	flag.Parse()

	connOpts := &MQTT.ClientOptions{
		ClientID:             *clientid,
		CleanSession:         true,
		Username:             *username,
		Password:             *password,
		MaxReconnectInterval: 1 * time.Second,
		KeepAlive:            int64(30 * time.Second),
		TLSConfig:            tls.Config{InsecureSkipVerify: true, ClientAuth: tls.NoClientCert},
	}
	connOpts.AddBroker(*server)
	connOpts.OnConnect = func(c MQTT.Client) {
		if token := c.Subscribe("thermometers/#", 0, onSensor); token.Wait() && token.Error() != nil {
			panic(token.Error())
		}
		if token := c.Subscribe("hygrometers/#", 0, onSensor); token.Wait() && token.Error() != nil {
			panic(token.Error())
		}
	}

	client := MQTT.NewClient(connOpts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	} else {
		fmt.Printf("Connected to %s\n", *server)
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}
