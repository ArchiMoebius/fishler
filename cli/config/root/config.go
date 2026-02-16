package config_root

import (
	"fmt"
	"os"

	"github.com/fatih/structs"
	"github.com/leebenson/conform"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Setting through a file, environment vars, and/or cli parameters.
var Setting *setting

var initial = &setting{
	LogBasepath:         "/var/log/fishler",
	Config:              ".fishler",
	Debug:               false,
	DockerImagename:     "fishler",
	DockerBasepath:      "docker",
	UplinkServerAddress: "",
}

// Create private data struct to hold setting options.
// `mapstructure` => viper tags
// `struct` => fatih structs tag
// `env` => environment variable name
type setting struct {
	LogBasepath         string `mapstructure:"log-basepath" structs:"log-basepath" env:"FISHLER_LOG_BASEPATH"`
	Config              string `mapstructure:"config" structs:"config" env:"FISHLER_CONFIG"`
	Debug               bool   `mapstructure:"debug" structs:"debug" env:"FISHLER_DEBUG"`
	DockerImagename     string `mapstructure:"docker-imagename" structs:"docker-imagename" env:"FISHLER_DOCKER_IMAGENAME"`
	DockerBasepath      string `mapstructure:"docker-basepath" structs:"docker-basepath" env:"FISHLER_DOCKER_BASEPATH"`
	UplinkServerAddress string `mapstructure:"uplink-server-address" structs:"uplink-server-address" env:"FISHLER_UPLINK_SERVER_ADDRESS"`
}

func Load() {
	// Priority of configuration options
	// 1: CLI Parameters
	// 2: environment
	// 3: config.yaml
	// 4: defaults
	// Create a map of the default config
	defaultsAsMap := structs.Map(initial)

	// Set defaults
	for key, value := range defaultsAsMap {
		viper.SetDefault(key, value)
	}
	// Read config from file
	viper.SetConfigName(".fishler")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	// Unmarshal config into struct
	Setting = &setting{}
	err := viper.Unmarshal(Setting)
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		os.Exit(1)
	}
}

// configInit must be called from the packages' init() func
func CommandInit(command *cobra.Command) error {
	// Keep cli parameters in sync with the config struct
	command.PersistentFlags().String("docker-imagename", initial.DockerImagename, "The image user for the docker container")
	command.PersistentFlags().String("docker-basepath", initial.DockerBasepath, "The path to the docker folder ./docker if run from the root of the project")
	command.PersistentFlags().String("uplink-server-address", initial.UplinkServerAddress, "The uplink server address in the form IP:PORT")
	command.PersistentFlags().StringP("log-basepath", "l", initial.LogBasepath, "The base filepath where logs will be stored")
	command.PersistentFlags().StringP("config", "c", initial.Config, ".fishler.yaml")
	command.PersistentFlags().BoolP("debug", "d", initial.Debug, "Output debug information")

	for _, field := range structs.Fields(&setting{}) {
		// Get the struct tag values
		key := field.Tag("structs")
		env := field.Tag("env")

		if key == "" {
			continue
		}

		// Bind cobra flags to viper
		err := viper.BindPFlag(key, command.PersistentFlags().Lookup(key))
		if err != nil {
			return err
		}
		err = viper.BindEnv(key, env)
		if err != nil {
			return err
		}
	}

	return nil
}

// Print the config object
// but remove sensitive data
func (c *setting) Print() {
	cp := *c
	_ = conform.Strings(&cp)
	litter.Dump(cp)
}

// String the config object
// but remove sensitive data
func (c *setting) String() string {
	cp := *c
	_ = conform.Strings(&cp)
	return litter.Sdump(cp)
}
