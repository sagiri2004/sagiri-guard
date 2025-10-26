package command

import "encoding/json"

type Kind string

const (
	KindOnce   Kind = "once"
	KindStream Kind = "stream"
)

type Envelope struct {
	DeviceID string          `json:"deviceid"`
	Name     string          `json:"command"`
	Kind     Kind            `json:"kind,omitempty"`
	Argument json.RawMessage `json:"argument,omitempty"`
}

type Handler interface {
	// Default kind of this command; used if envelope.Kind is empty
	Kind() Kind
	// DecodeArg lets each command define its own argument struct (or nil)
	DecodeArg(raw json.RawMessage) (any, error)
	// HandleOnce executes a one-off command; only used when kind==once
	HandleOnce(arg any) error
	// Start starts a continuous task; only used when kind==stream
	Start(arg any) (stop func() error, err error)
}

// Registry maps command name to handler
var registry = map[string]Handler{}

func Register(name string, h Handler) { registry[name] = h }

func Get(name string) (Handler, bool) { h, ok := registry[name]; return h, ok }
