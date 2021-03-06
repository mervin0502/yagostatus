package config

import (
	"encoding/json"
	"io/ioutil"
	"strings"

	"github.com/burik666/yagostatus/ygs"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Config represents the main configuration.
type Config struct {
	Widgets []WidgetConfig `yaml:"widgets"`
}

// WidgetConfig represents a widget configuration.
type WidgetConfig struct {
	Name       string              `yaml:"widget"`
	Workspaces []string            `yaml:"workspaces"`
	Template   ygs.I3BarBlock      `yaml:"-"`
	Events     []WidgetEventConfig `yaml:"events"`

	Params map[string]interface{}
}

// Validate checks widget configuration.
func (c WidgetConfig) Validate() error {
	if c.Name == "" {
		return errors.New("Missing widget name")
	}
	for _, e := range c.Events {
		if err := e.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// WidgetEventConfig represents a widget events.
type WidgetEventConfig struct {
	Command   string   `yaml:"command"`
	Button    uint8    `yaml:"button"`
	Modifiers []string `yaml:"modifiers,omitempty"`
	Name      string   `yaml:"name,omitempty"`
	Instance  string   `yaml:"instance,omitempty"`
	Output    bool     `yaml:"output,omitempty"`
}

// Validate checks event parameters.
func (e WidgetEventConfig) Validate() error {
	var availableWidgetEventModifiers = [...]string{"Shift", "Control", "Mod1", "Mod2", "Mod3", "Mod4", "Mod5"}
	for _, mod := range e.Modifiers {
		found := false
		mod = strings.TrimLeft(mod, "!")
		for _, m := range availableWidgetEventModifiers {
			if mod == m {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("Unknown '%s' modifier", mod)
		}
	}
	return nil
}

// LoadFile loads and parses config from file.
func LoadFile(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses config.
func Parse(data []byte) (*Config, error) {

	var raw struct {
		Widgets []map[string]interface{} `yaml:"widgets"`
	}

	config := Config{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	for widgetIndex := range config.Widgets {
		widget := &config.Widgets[widgetIndex]
		params := raw.Widgets[widgetIndex]

		tpl, ok := params["template"]
		if ok {
			if err := json.Unmarshal([]byte(tpl.(string)), &widget.Template); err != nil {
				return nil, err
			}
		}

		widget.Params = params
		if err := widget.Validate(); err != nil {
			return nil, err
		}

		delete(params, "widget")
		delete(params, "workspaces")
		delete(params, "template")
		delete(params, "events")
	}
	return &config, nil
}
