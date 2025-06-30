package executor

import (
	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/config"
)

var LoggerConfigKey = "code_executor"

type ConfigCodeExecutors struct {
	Docker map[string]*ConfigCodeExecutorDocker `mapstructure:"docker"`
	SSH    map[string]*ConfigCodeExecutorSSH    `mapstructure:"ssh"`
	Local  map[string]*ConfigCodeExecutorLocal  `mapstructure:"local"`
}

type CodeExecutorCommonConfig struct {
	ID               uuid.UUID `gorm:"type:uuid;primary_key"`
	Name             string    `yaml:"name,omitempty"`
	CodeExecutorPath string    `mapstructure:"code_executor_path"`
	TimeoutSec       float64   `mapstructure:"timeout_sec"default:"30"`
}

type ConfigCodeExecutorDocker struct {
	CodeExecutorCommonConfig `mapstructure:",squash"` // squash the common config into the docker config
	Image                    string                   `mapstructure:"image"`
	MemLimitMB               float64                  `mapstructure:"mem_limit_mb" default:"256"`
	CPUsLimit                float64                  `mapstructure:"cpus_limit_cores" default:"1"`
	PIDsLimit                int64                    `mapstructure:"pids_limit" default:"50"`
}

func (c *ConfigCodeExecutorDocker) ExecutorConfigEqual(other *ConfigCodeExecutorDocker) bool {
	return c.Name == other.Name &&
		c.CodeExecutorPath == other.CodeExecutorPath &&
		c.TimeoutSec == other.TimeoutSec &&
		c.Image == other.Image &&
		c.MemLimitMB == other.MemLimitMB &&
		c.CPUsLimit == other.CPUsLimit &&
		c.PIDsLimit == other.PIDsLimit
}

type ConfigCodeExecutorSSH struct {
	CodeExecutorCommonConfig `mapstructure:",squash"` // squash the common config into the ssh config
	Host                     string                   `mapstructure:"host" yaml:"host,omitempty"`
	Port                     int                      `mapstructure:"port" yaml:"port,omitempty" default:"22"`
	Username                 string                   `mapstructure:"username" yaml:"username,omitempty"`
	Password                 string                   `mapstructure:"password" yaml:"password,omitempty"`
	PrivateKey               string                   `mapstructure:"private_key" yaml:"private_key,omitempty"`
}

func (c *ConfigCodeExecutorSSH) ExecutorConfigEqual(other *ConfigCodeExecutorSSH) bool {
	return c.Name == other.Name &&
		c.CodeExecutorPath == other.CodeExecutorPath &&
		c.TimeoutSec == other.TimeoutSec &&
		c.Host == other.Host &&
		c.Port == other.Port &&
		c.Username == other.Username &&
		c.Password == other.Password &&
		c.PrivateKey == other.PrivateKey
}

type ConfigCodeExecutorLocal struct {
	CodeExecutorCommonConfig `mapstructure:",squash"` // squash the common config into the local config
}

func (c *ConfigCodeExecutorLocal) ExecutorConfigEqual(other *ConfigCodeExecutorLocal) bool {
	return c.Name == other.Name &&
		c.CodeExecutorPath == other.CodeExecutorPath &&
		c.TimeoutSec == other.TimeoutSec
}

// Add these methods to ConfigCodeExecutors to implement ConfigComponent
func (c *ConfigCodeExecutors) GetDefault() interface{} {
	return c
}

func (c *ConfigCodeExecutors) Load(name string, configDict map[string]interface{}) error {
	err := config.UpdateStructFromConfig(c, configDict)
	if err != nil {
		return err
	}

	/*
		if c.Docker != nil {
			if c.Docker == nil {
				c.Docker = make(map[string]*ConfigCodeExecutorDocker)
			}
			for k, v := range c.Docker {
				c.Docker[k] = v
			}
		}

		if c.SSH != nil {
			if c.SSH == nil {
				c.SSH = make(map[string]*ConfigCodeExecutorSSH)
			}
			for k, v := range c.SSH {
				c.SSH[k] = v
			}
		}

		if config.Local != nil {
			if c.Local == nil {
				c.Local = make(map[string]*ConfigCodeExecutorLocal)
			}
			for k, v := range config.Local {
				c.Local[k] = v
			}
		}
	*/
	return nil
}

var executorConfig = &ConfigCodeExecutors{
	Docker: map[string]*ConfigCodeExecutorDocker{
		"docker_python_default": {
			CodeExecutorCommonConfig: CodeExecutorCommonConfig{
				CodeExecutorPath: "/usr/local/bin/python3.11",
				TimeoutSec:       30,
			},
			Image:      "python:3.11-alpine",
			MemLimitMB: 1024,
			CPUsLimit:  1,
			PIDsLimit:  50,
		},
	},
	SSH: map[string]*ConfigCodeExecutorSSH{
		"ssh_python_default": {
			Host:       "localhost",
			Port:       22,
			Username:   "user",
			Password:   "password",
			PrivateKey: "~/.ssh/id_rsa",
			CodeExecutorCommonConfig: CodeExecutorCommonConfig{
				CodeExecutorPath: "/usr/local/bin/python3.11",
				TimeoutSec:       30,
			},
		},
	},
	Local: map[string]*ConfigCodeExecutorLocal{
		"local_python_default": {
			CodeExecutorCommonConfig: CodeExecutorCommonConfig{
				CodeExecutorPath: "/usr/local/bin/python3.11",
				TimeoutSec:       30,
			},
		},
	},
}

func init() {
	config.RegisterConfigComponent("code_executors", executorConfig)
}
