package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
)

func init() {
	logo := `
   ___           ________  ______
  / _ \___ _  __/  _/ __ \/_  __/
 / // / -_) |/ // // /_/ / / /
/____/\__/|___/___/\____/ /_/`
	println(logo)
}

// Connector represents a connector
type Connector interface {
	Start() error
	Stop() error
	Publish(data map[string]interface{}) error
}

// Gateway represents gateway service
type Gateway struct {
	Name         string             `json:"name"`
	Kind         string             `json:"kind,omitempty"`
	Host         string             `json:"host,omitempty"`
	Port         int                `json:"port,omitempty"`
	Data         string             `json:"data,omitempty"`
	Action       string             `json:"action,omitempty"`
	Account      string             `json:"owner,omitempty"`
	Mode         int                `json:"mode"`
	Things       map[string]wrapper `json:"sensors,omitempty"`
	Logger       *log.Logger
	connector    Connector
	deviotServer string
	registered   int32
}

// GatewayModeHttpPull connect to DevIoT using pull based HTTP protocol
const GatewayModeHttpPull = 0

// GatewayModeHttpPush connect to DevIoT using push based HTTP protocol
const GatewayModeHttpPush = 1

// GatewayModeMqtt connect to DevIoT using MQTT protocol
const GatewayModeMqtt = 2

type wrapper struct {
	thing    Thing
	instance Instance
}

// NewGateway create a gateway service
func NewGateway(name string, deviotServer string, connectorServer string, account string, opts map[string]interface{}) (*Gateway, error) {
	name = strings.Replace(name, "-", "_", -1)

	gateway := Gateway{
		Name:         name,
		Account:      account,
		Kind:         "device",
		Mode:         GatewayModeMqtt,
		Logger:       log.New(os.Stdout, "[DevIoT] ", log.LstdFlags),
		Things:       make(map[string]wrapper),
		deviotServer: deviotServer,
	}
	if opts != nil {
		kind, found := opts["kind"]
		if found {
			gateway.Kind = kind.(string)
		}
	}
	connector, err := NewMqttConnector(&gateway, connectorServer)
	if err != nil {
		return nil, err
	}
	gateway.connector = connector
	return &gateway, nil
}

// Start gateway service and register it to DevIoT server
func (g *Gateway) Start() error {
	err := g.connector.Start()
	if err != nil {
		return err
	}
	go func() {
		model := make(map[string]interface{})
		model["name"] = g.Name
		model["kind"] = g.Kind
		model["mode"] = g.Mode
		model["owner"] = g.Account
		model["host"] = g.Host
		model["port"] = g.Port
		model["data"] = g.Data
		model["action"] = g.Action

		first := true
		for {
			sensors := make([]interface{}, 0)
			for _, w := range g.Things {
				sensors = append(sensors, w.thing)
			}
			model["sensors"] = sensors
			jsonData, _ := json.Marshal(model)

			url := fmt.Sprintf("%s%s", g.deviotServer, "/api/v1/gateways")
			req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)

			if err != nil {
				if first || g.IsRegistered() {
					g.Logger.Printf("Failed to register gateway service %s - %v", g.Name, err)
				}
				g.setRegistered(false)
			} else {
				resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 400 {
					if first || !g.IsRegistered() {
						g.Logger.Printf("Registered gateway service to %s", g.deviotServer)
					}
					g.setRegistered(true)
				} else {
					if first || g.IsRegistered() {
						g.Logger.Printf("Failed to register gateway service %s - %v", g.Name, resp.Status)
					}
					g.setRegistered(false)
				}
			}

			first = false
			time.Sleep(1 * time.Minute)
		}
	}()
	g.Logger.Printf("gateway service %s started", g.Name)
	return err
}

// Stop gateway service
func (g *Gateway) Stop() error {
	g.Logger.Printf("gateway service %s stopped", g.Name)
	return g.connector.Stop()
}

// Register a thing to gateway service
func (g *Gateway) Register(id string, name string, instance Instance) {
	if _, ok := g.Things[id]; ok {
		g.Logger.Printf("Thing %s has already been registered", id)
	} else {
		kind := strings.ToLower(reflect.TypeOf(instance).Elem().String())
		parts := strings.Split(kind, ".")
		kind = parts[len(parts)-1]
		t := Thing{Id: id, Name: name, Kind: kind}
		t.Actions = make([]Action, 0)
		t.Properties = make([]Property, 0)
		instance.Init(&t)
		g.Things[id] = wrapper{thing: t, instance: instance}
		g.Logger.Printf("Thing %s.%s(%v) registered", t.Id, t.Name, t.Kind)
	}
}

// IsRegistered check if gateway service has been registered to DevIoT
func (g *Gateway) IsRegistered() bool {
	v := atomic.LoadInt32(&g.registered)
	return v == 1
}

func (g *Gateway) setRegistered(registered bool) {
	if registered {
		atomic.StoreInt32(&g.registered, 1)
	} else {
		atomic.StoreInt32(&g.registered, 0)
	}
}

// Deregister a thing from gateway service
func (g *Gateway) Deregister(id string) {
	if w, ok := g.Things[id]; ok {
		delete(g.Things, id)
		t := w.thing
		g.Logger.Printf("Thing %s.%s(%s) deregistered", id, t.Name, t.Kind)
	} else {
		g.Logger.Printf("Thing %s not registered yet", id)
	}
}

// SendData send data to DevIoT server
func (g *Gateway) SendData(data map[string]interface{}) error {
	return g.connector.Publish(data)
}

// CallAction call an action on thing
func (g *Gateway) CallAction(data map[string]interface{}) {
	id, found := data["id"]
	if !found {
		id, found = data["name"]
	}
	if !found {
		g.Logger.Printf("Illegal message, thing id/name(%v) not available", id)
		return
	}
	wrapper, found := g.Things[id.(string)]
	if !found {
		g.Logger.Printf("Illegal message, thing id/name(%v) not available", id)
		return
	}

	action, found := data["action"]
	if !found {
		g.Logger.Printf("Illegal message, thing action(%v) not available", action)
		return
	}
	actionDef, found := wrapper.thing.FindAction(action.(string))
	if !found {
		g.Logger.Printf("Illegal message, thing action(%v) not available", action)
		return
	}

	method, ok := reflect.TypeOf(wrapper.instance).MethodByName(action.(string))
	if !ok {
		g.Logger.Printf("Illegal message, thing method(%v) not available", action)
		return
	}

	if method.Type.NumIn() != len(actionDef.Parameters)+1 && method.Type.NumIn() != len(actionDef.Parameters)+2 {
		g.Logger.Printf("Illegal message, thing method(%v) arguments(%d:%d) does not match",
			action, method.Type.NumIn(), len(actionDef.Parameters))
		return
	}

	args := make([]reflect.Value, method.Type.NumIn())
	args[0] = reflect.ValueOf(wrapper.instance)
	for i := 1; i < len(args); i++ {
		param := actionDef.Parameters[i-1]
		v, found := data[param.Name]
		if !found {
			// use default value
			args[i] = reflect.ValueOf(param.Value)
		} else {
			v, err := convert(param.Type, method.Type.In(i), v)
			if err != nil {
				g.Logger.Printf("%v", err)
				return
			}
			args[i] = reflect.ValueOf(v)
		}
	}
	if method.Type.NumIn() == len(actionDef.Parameters)+2 {
		payload, found := data["payload"]
		if found {
			args[len(args)-1] = reflect.ValueOf(payload)
		}
	}
	method.Func.Call(args)
}

func convert(param int, target reflect.Type, value interface{}) (result interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()
	switch target.Kind() {
	case reflect.Int:
		return int(value.(float64)), nil
	case reflect.Int8:
		return int8(value.(float64)), nil
	case reflect.Int16:
		return int16(value.(float64)), nil
	case reflect.Int32:
		return int32(value.(float64)), nil
	case reflect.Int64:
		return int64(value.(float64)), nil
	case reflect.Uint:
		return uint(value.(float64)), nil
	case reflect.Uint8:
		return uint8(value.(float64)), nil
	case reflect.Uint16:
		return uint16(value.(float64)), nil
	case reflect.Uint32:
		return uint32(value.(float64)), nil
	case reflect.Uint64:
		return uint64(value.(float64)), nil
	case reflect.Float32:
		return float32(value.(float64)), nil
	case reflect.Float64:
		return float64(value.(float64)), nil
	case reflect.String:
		return fmt.Sprintf("%v", value), nil
	}
	return nil, fmt.Errorf("expected type: %v, got %v", target, reflect.TypeOf(value))
}
