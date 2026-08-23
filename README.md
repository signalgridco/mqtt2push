# mqtt2push

Turn MQTT messages into push notifications.

``` text
MQTT Broker → mqtt2push → Push Notification → iOS / Android
```

`mqtt2push` is a lightweight bridge that subscribes to one or more MQTT
topics and forwards incoming messages as push notifications to your
phone.

The MQTT topic becomes the notification title and the MQTT payload
becomes the notification body.

For example:

``` text
Topic:
factory/production/temperature

Message:
Temperature exceeded 80°C
```

becomes:

``` text
factory · production · temperature

Temperature exceeded 80°C
```

## Features

-   Subscribe to multiple MQTT topics
-   MQTT wildcard support
-   MQTT QoS 0, 1 and 2
-   Username/password authentication
-   Plain MQTT
-   MQTT over TLS
-   Custom CA certificates
-   Automatic reconnect
-   Automatic re-subscription after reconnect
-   Lightweight standalone binary
-   No inbound ports required
-   iOS and Android push notifications
-   Open source

## Installation

Go to /usr/src:

``` bash
cd /usr/src
```

Download the current git repository:

``` bash
git clone git@github.com:signalgridco/mqtt2push.git
```

Download the appropriate `mqtt2push` binary for your system from the
GitHub Releases page.

Make it executable:

``` bash
chmod +x mqtt2push
```

Create a `config.yml` next to the executable and start it:

``` bash
./mqtt2push
```

## Configuration

### MQTT without TLS

``` yaml
mqtt:
  broker: "mqtt.example.com"
  port: 1883
  topics:
    - "alerts/#"
    - "servers/+/status"
  username: "mqttuser"
  password: "mqttpassword"
  qos: 0
  client_id: "mqtt2push"
  tls:
    enabled: false

signalgrid:
  client_key: "YOUR_CLIENT_KEY"
  channel: "YOUR_CHANNEL"
  type: "INFO"
```

When TLS is disabled, the connection is made using plain MQTT over TCP.
If `port` is omitted, `1883` is used automatically.

### MQTT with TLS

``` yaml
mqtt:
  broker: "mqtt.example.com"
  port: 8883
  topics:
    - "alerts/#"
    - "servers/+/status"
  username: "mqttuser"
  password: "mqttpassword"
  qos: 0
  client_id: "mqtt2push"
  tls:
    enabled: true

signalgrid:
  client_key: "YOUR_CLIENT_KEY"
  channel: "YOUR_CHANNEL"
  type: "INFO"
```

When TLS is enabled without a custom CA file, `mqtt2push` uses the
operating system's normal trusted CA store. If `port` is omitted, `8883`
is used automatically.

### MQTT with TLS and a custom CA

``` yaml
mqtt:
  broker: "mqtt.internal.example"
  port: 8883
  topics:
    - "factory/alarms/#"
  username: "mqttuser"
  password: "mqttpassword"
  qos: 1
  client_id: "mqtt2push"
  tls:
    enabled: true
    ca_file: "/etc/mqtt2push/company-ca.pem"

signalgrid:
  client_key: "YOUR_CLIENT_KEY"
  channel: "YOUR_CHANNEL"
  type: "INFO"
```

The custom CA certificate is added to the operating system's existing
trusted CA store.

## Push Delivery

Push notification delivery is provided by
[Signalgrid](https://signalgrid.co).

A Signalgrid account and channel are required to obtain the `client_key`
and `channel` values used in `config.yml`.

Once configured, `mqtt2push` receives MQTT messages from your broker and
forwards them to the configured Signalgrid channel for delivery to your
devices.

## Topics

Multiple MQTT subscriptions can be configured:

``` yaml
topics:
  - "factory/alarms/#"
  - "servers/+/status"
  - "network/core/events"
```

Normal MQTT wildcards are supported.

## Notification Mapping

`mqtt2push` does not interpret or transform the MQTT payload.

The MQTT topic:

``` text
factory/line1/alarm
```

is used as the notification title and displayed as:

``` text
factory · line1 · alarm
```

The MQTT payload:

``` text
Motor temperature exceeded 80°C
```

is used directly as the notification body.

## Running

Place `config.yml` next to the executable:

``` text
mqtt2push
config.yml
```

Start the bridge:

``` bash
./mqtt2push
```

Example output:

``` text
Connected to MQTT broker mqtt.example.com:1883
Subscribed to alerts/# with QoS 0
Topic: alerts/server1 | Message: Server unavailable | Response: 200 OK
```

## Building from Source

Requires Go.

``` bash
go mod tidy
go build -o mqtt2push
```

Then:

``` bash
./mqtt2push
```
