package command

import (
	"fmt"
	"sagiri-guard/agent/internal/logger"
	"sync"
)

// Manager keeps running stream commands
type Manager struct {
	mu     sync.Mutex
	active map[string]func() error // name->stop
}

func NewManager() *Manager { return &Manager{active: map[string]func() error{}} }

// Format renders a human-friendly string of the command envelope
func Format(env Envelope) string {
	return fmt.Sprintf("command=%s kind=%s device=%s", env.Name, resolveKind(env), env.DeviceID)
}

func resolveKind(env Envelope) Kind {
	if env.Kind != "" {
		return env.Kind
	}
	if h, ok := Get(env.Name); ok {
		return h.Kind()
	}
	return KindOnce
}

// Dispatch executes or starts the given command and logs outcome
func (m *Manager) Dispatch(env Envelope) {
	h, ok := Get(env.Name)
	if !ok {
		logger.Errorf("Unknown command: %s", env.Name)
		return
	}
	k := resolveKind(env)
	var arg any
	if len(env.Argument) > 0 {
		var err error
		arg, err = h.DecodeArg(env.Argument)
		if err != nil {
			logger.Errorf("Decode arg failed for %s: %v", env.Name, err)
			return
		}
	}
	logger.Infof("Received command=%s kind=%s device=%s", env.Name, k, env.DeviceID)
	switch k {
	case KindOnce:
		if err := h.HandleOnce(arg); err != nil {
			logger.Errorf("Command %s failed: %v", env.Name, err)
		} else {
			logger.Infof("Command %s completed", env.Name)
		}
	case KindStream:
		m.mu.Lock()
		if stop, exists := m.active[env.Name]; exists {
			_ = stop()
			delete(m.active, env.Name)
		}
		m.mu.Unlock()
		stop, err := h.Start(arg)
		if err != nil {
			logger.Errorf("Start %s failed: %v", env.Name, err)
			return
		}
		m.mu.Lock()
		m.active[env.Name] = stop
		m.mu.Unlock()
		logger.Infof("Command %s started", env.Name)
	}
}

// Stop stops a running stream command by name (if exists)
func (m *Manager) Stop(name string) {
	m.mu.Lock()
	stop, exists := m.active[name]
	if exists {
		delete(m.active, name)
	}
	m.mu.Unlock()
	if exists {
		_ = stop()
		logger.Infof("Command %s stopped", name)
	}
}
