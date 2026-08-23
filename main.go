package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gopkg.in/yaml.v3"
)

type Config struct {
	MQTT struct {
		Broker   string   `yaml:"broker"`
		Port     int      `yaml:"port"`
		Topics   []string `yaml:"topics"`
		Username string   `yaml:"username"`
		Password string   `yaml:"password"`
		QoS      byte     `yaml:"qos"`
		ClientID string   `yaml:"client_id"`

		TLS struct {
			Enabled bool   `yaml:"enabled"`
			CAFile  string `yaml:"ca_file"`
		} `yaml:"tls"`
	} `yaml:"mqtt"`

	Signalgrid struct {
		ClientKey string `yaml:"client_key"`
		Channel   string `yaml:"channel"`
		Type      string `yaml:"type"`
	} `yaml:"signalgrid"`
}

type Message struct {
	Topic   string
	Payload []byte
}

type SignalgridResponse struct {
	Text string `json:"text"`
	Code string `json:"code"`
}

func main() {
	cfg := loadConfig("config.yml")

	queue := make(chan Message, 100)

	go signalgridWorker(cfg, queue)

	brokerURL := buildBrokerURL(cfg)

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(cfg.MQTT.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(30 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second).
		SetOrderMatters(false)

	if cfg.MQTT.Username != "" {
		opts.SetUsername(cfg.MQTT.Username)
	}

	if cfg.MQTT.Password != "" {
		opts.SetPassword(cfg.MQTT.Password)
	}

	if cfg.MQTT.TLS.Enabled {
		tlsConfig, err := buildTLSConfig(cfg.MQTT.TLS.CAFile)

		if err != nil {
			log.Fatalf("TLS configuration failed: %v", err)
		}

		opts.SetTLSConfig(tlsConfig)

		if cfg.MQTT.TLS.CAFile != "" {
			log.Printf(
				"MQTT TLS enabled with custom CA: %s",
				cfg.MQTT.TLS.CAFile,
			)
		} else {
			log.Printf(
				"MQTT TLS enabled using system CA store",
			)
		}
	}

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	})

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Printf(
			"Connected to MQTT broker %s:%d",
			cfg.MQTT.Broker,
			cfg.MQTT.Port,
		)

		for _, topic := range cfg.MQTT.Topics {
			token := client.Subscribe(
				topic,
				cfg.MQTT.QoS,
				func(client mqtt.Client, msg mqtt.Message) {
					payload := append([]byte(nil), msg.Payload()...)

					select {
					case queue <- Message{
						Topic:   msg.Topic(),
						Payload: payload,
					}:

					default:
						log.Printf(
							"Queue full, dropping MQTT message on topic %s",
							msg.Topic(),
						)
					}
				},
			)

			token.Wait()

			if err := token.Error(); err != nil {
				log.Printf(
					"MQTT subscribe failed for %s: %v",
					topic,
					err,
				)

				continue
			}

			log.Printf(
				"Subscribed to %s with QoS %d",
				topic,
				cfg.MQTT.QoS,
			)
		}
	})

	client := mqtt.NewClient(opts)

	token := client.Connect()
	token.Wait()

	if err := token.Error(); err != nil {
		log.Fatalf("MQTT connection failed: %v", err)
	}

	sig := make(chan os.Signal, 1)

	signal.Notify(
		sig,
		syscall.SIGINT,
		syscall.SIGTERM,
	)

	<-sig

	log.Println("Shutting down")

	client.Disconnect(250)
}

func buildBrokerURL(cfg Config) string {
	scheme := "tcp"

	if cfg.MQTT.TLS.Enabled {
		scheme = "ssl"
	}

	return fmt.Sprintf(
		"%s://%s:%d",
		scheme,
		cfg.MQTT.Broker,
		cfg.MQTT.Port,
	)
}

func buildTLSConfig(caFile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if caFile == "" {
		return tlsConfig, nil
	}

	systemPool, err := x509.SystemCertPool()

	if err != nil || systemPool == nil {
		systemPool = x509.NewCertPool()
	}

	caData, err := os.ReadFile(caFile)

	if err != nil {
		return nil, fmt.Errorf(
			"cannot read CA file %s: %w",
			caFile,
			err,
		)
	}

	if ok := systemPool.AppendCertsFromPEM(caData); !ok {
		return nil, fmt.Errorf(
			"no valid certificates found in CA file %s",
			caFile,
		)
	}

	tlsConfig.RootCAs = systemPool

	return tlsConfig, nil
}

func loadConfig(filename string) Config {
	data, err := os.ReadFile(filename)

	if err != nil {
		log.Fatalf("Cannot read %s: %v", filename, err)
	}

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	cfg.MQTT.Broker = strings.TrimSpace(cfg.MQTT.Broker)

	if cfg.MQTT.Broker == "" {
		log.Fatal("mqtt.broker is required")
	}

	if cfg.MQTT.Port == 0 {
		if cfg.MQTT.TLS.Enabled {
			cfg.MQTT.Port = 8883
		} else {
			cfg.MQTT.Port = 1883
		}
	}

	if cfg.MQTT.Port < 1 || cfg.MQTT.Port > 65535 {
		log.Fatal("mqtt.port must be between 1 and 65535")
	}

	if len(cfg.MQTT.Topics) == 0 {
		log.Fatal("mqtt.topics requires at least one topic")
	}

	for i, topic := range cfg.MQTT.Topics {
		cfg.MQTT.Topics[i] = strings.TrimSpace(topic)

		if cfg.MQTT.Topics[i] == "" {
			log.Fatal("mqtt.topics cannot contain an empty topic")
		}
	}

	if cfg.MQTT.ClientID == "" {
		cfg.MQTT.ClientID = "mqtt2push"
	}

	if cfg.MQTT.QoS > 2 {
		log.Fatal("mqtt.qos must be 0, 1 or 2")
	}

	if cfg.Signalgrid.ClientKey == "" {
		log.Fatal("signalgrid.client_key is required")
	}

	if cfg.Signalgrid.Channel == "" {
		log.Fatal("signalgrid.channel is required")
	}

	if cfg.Signalgrid.Type == "" {
		cfg.Signalgrid.Type = "INFO"
	}

	cfg.Signalgrid.Type = strings.ToUpper(
		cfg.Signalgrid.Type,
	)

	return cfg
}

func signalgridWorker(cfg Config, queue <-chan Message) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	for msg := range queue {
		title := strings.ReplaceAll(
			msg.Topic,
			"/",
			" · ",
		)

		body := string(msg.Payload)

		err := sendSignalgrid(
			client,
			cfg,
			msg.Topic,
			title,
			body,
		)

		if err != nil {
			log.Printf(
				"Topic: %s | Message: %s | Error: %v",
				msg.Topic,
				body,
				err,
			)
		}
	}
}

func sendSignalgrid(
	client *http.Client,
	cfg Config,
	topic string,
	title string,
	body string,
) error {
	form := url.Values{}

	form.Set(
		"client_key",
		cfg.Signalgrid.ClientKey,
	)

	form.Set(
		"channel",
		cfg.Signalgrid.Channel,
	)

	form.Set(
		"title",
		title,
	)

	form.Set(
		"body",
		body,
	)

	form.Set(
		"type",
		cfg.Signalgrid.Type,
	)

	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.signalgrid.co/v1/push",
		strings.NewReader(form.Encode()),
	)

	if err != nil {
		return err
	}

	req.Header.Set(
		"Content-Type",
		"application/x-www-form-urlencoded",
	)

	req.Header.Set(
		"User-Agent",
		"mqtt2push/0.1",
	)

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	responseBody, err := io.ReadAll(
		io.LimitReader(resp.Body, 4096),
	)

	if err != nil {
		return err
	}

	var sgResponse SignalgridResponse

	if err := json.Unmarshal(
		responseBody,
		&sgResponse,
	); err != nil {

		log.Printf(
			"Topic: %s | Message: %s | Response: HTTP %d %s",
			topic,
			body,
			resp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)

	} else {
		log.Printf(
			"Topic: %s | Message: %s | Response: %s %s",
			topic,
			body,
			sgResponse.Code,
			sgResponse.Text,
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	return nil
}
