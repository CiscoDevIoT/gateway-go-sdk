package gateway

const (
	// PropertyTypeInt represents number type of a property
	PropertyTypeInt = 0
	// PropertyTypeString represents string type of a property
	PropertyTypeString = 1
	// PropertyTypeBool represents boolean type of a property
	PropertyTypeBool = 2
	// PropertyTypeColor represents color type of a property
	PropertyTypeColor = 3
)

// Instance represents a thing instance can be registered to DevIoT
type Instance interface {
	Init(thing *Thing)
}

// Property represents a property model in DevIoT
type Property struct {
	Name        string        `json:"name"`
	Type        int           `json:"type"`
	Value       interface{}   `json:"value,omitempty"`
	Range       []interface{} `json:"range,omitempty"`
	Unit        string        `json:"unit,omitempty"`
	Description string        `json:"description,omitempty"`
}

// Action represents a action model in DevIoT
type Action struct {
	Name       string     `json:"name,omitempty"`
	Parameters []Property `json:"parameters,omitempty"`
}

// Thing represents a thing model in DevIoT
type Thing struct {
	Id         string     `json:"id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind,omitempty"`
	Actions    []Action   `json:"actions,omitempty"`
	Properties []Property `json:"properties,omitempty"`
}

// AddParameter add a parameter to action model
func (a *Action) AddParameter(p Property) *Action {
	a.Parameters = append(a.Parameters, p)
	return a
}

// AddAction add an action to thing model
func (t *Thing) AddAction(a Action) *Thing {
	t.Actions = append(t.Actions, a)
	return t
}

// AddProperty add a property to thing model
func (t *Thing) AddProperty(p Property) *Thing {
	t.Properties = append(t.Properties, p)
	return t
}

// FindAction find action definition by name
func (t *Thing) FindAction(name string) (*Action, bool) {
	for _, a := range t.Actions {
		if a.Name == name {
			return &a, true
		}
	}
	return nil, false
}
