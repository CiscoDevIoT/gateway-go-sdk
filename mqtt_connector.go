package gateway

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// MqttConnector represents mqtt based connector
type MqttConnector struct {
	DataTopic       string
	ActionTopic     string
	connectorServer string
	client          mqtt.Client
	gateway         *Gateway
}

const defaultPort = 1883

// NewMqttConnector create a mqtt based connector
func NewMqttConnector(gateway *Gateway, connectorServer string) (*MqttConnector, error) {
	u, err := url.Parse(connectorServer)
	if err != nil {
		return nil, err
	}
	host, portStr, _ := net.SplitHostPort(u.Host)
	var port int
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, err
		}
	} else {
		port = defaultPort
		host = u.Host
	}

	ns := strings.Replace(gateway.Account, "@", "", -1)
	if ns == "" {
		ns = "_"
	}

	dataTopic := fmt.Sprintf("/deviot/%s/%s/data", ns, gateway.Name)
	actionTopic := fmt.Sprintf("/deviot/%s/%s/action", ns, gateway.Name)

	gateway.Host = host
	gateway.Port = port
	gateway.Mode = GatewayModeMqtt
	gateway.Data = dataTopic
	gateway.Action = actionTopic

	mqtt.ERROR = gateway.Logger
	mqtt.CRITICAL = gateway.Logger

	return &MqttConnector{
		DataTopic:       dataTopic,
		ActionTopic:     actionTopic,
		connectorServer: connectorServer,
		gateway:         gateway,
	}, nil
}

// Start mqtt connector
func (c *MqttConnector) Start() error {
	opts := mqtt.NewClientOptions().AddBroker(c.connectorServer).
		SetClientID(c.gateway.Name).
		SetAutoReconnect(true).
		SetCleanSession(false)
	opts.OnConnect = func(client mqtt.Client) {
		if token := c.client.Subscribe(c.ActionTopic, 0, c.onMessage); token.Wait() && token.Error() != nil {
			c.gateway.Logger.Printf("Failed to subscribe to topic %s - %v", c.ActionTopic, token.Error())
		}
	}
	c.client = mqtt.NewClient(opts)
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	c.gateway.Logger.Printf("mqtt server %s connected", c.connectorServer)
	return nil
}

// Stop mqtt connector
func (c *MqttConnector) Stop() error {
	c.client.Disconnect(250)
	return nil
}

// Publish to DevioT server
func (c *MqttConnector) Publish(data map[string]interface{}) error {
	if c.client != nil && c.client.IsConnected() {
		jsonData, _ := json.Marshal(data)
		token := c.client.Publish(c.DataTopic, 0, false, string(jsonData))
		token.Wait()
		return token.Error()
	}
	return nil
}

func (c *MqttConnector) onMessage(client mqtt.Client, msg mqtt.Message) {
	var data map[string]interface{}
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		c.gateway.Logger.Printf("Failed to unmarshal message %s - %v", string(msg.Payload()), err)
	} else {
		c.gateway.CallAction(data)
	}
}
