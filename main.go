package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type config struct {
	Name  string `json:"name"`
	Topic string `json:"topic"`
	Area  string `json:"area"`
	Floor string `json:"floor,omitempty"`
}

var temperature = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "thermometer_temperature_celsius",
		Help: "Current temperature of the thermometer.",
	},
	[]string{"sensor_name", "area", "floor"},
)

var humidity = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "hygrometer_humidity_percent",
		Help: "Current humidity of the hygrometer.",
	},
	[]string{"sensor_name", "area", "floor"},
)

func init() {
	prometheus.MustRegister(temperature)
	prometheus.MustRegister(humidity)
}

func onConfig(client MQTT.Client, message MQTT.Message) {
	// {hygrometers,thermometers}/sovrum/config
	parts := strings.SplitN(message.Topic(), "/", 3)
	name := parts[1]
	fmt.Printf("Config: %s(%s)\n", parts[0], name)
	var config config
	json.Unmarshal(message.Payload(), &config)
	client.Subscribe(parts[0]+"/"+name+"/value", 0, func(client MQTT.Client, message MQTT.Message) {
		value, err := strconv.ParseFloat(string(message.Payload()), 64)
		if err == nil {
			fmt.Printf("Value: %s(%s) = %.1f\n", parts[0], name, value)
			if parts[0] == "thermometers" {
				temperature.WithLabelValues(name, config.Area, config.Floor).Set(value)
			} else if parts[0] == "hygrometers" {
				humidity.WithLabelValues(name, config.Area, config.Floor).Set(value)
			}
		}
	})
}

func main() {
	runtime.GOMAXPROCS(1)
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
		if token := c.Subscribe("thermometers/+/config", 0, onConfig); token.Wait() && token.Error() != nil {
			panic(token.Error())
		}
		if token := c.Subscribe("hygrometers/+/config", 0, onConfig); token.Wait() && token.Error() != nil {
			panic(token.Error())
		}
	}
	connOpts.OnConnectionLost = func(c MQTT.Client, err error) {
		fmt.Printf("Disconnected from broker: %v - quitting\n", err)
		os.Exit(1)
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
