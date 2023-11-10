package config

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/fatih/structs"
	"github.com/leebenson/conform"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// handle global configuration through a config file, environment vars, and/or cli parameters.

var accounts = make(map[string][]string)
var passwords = make(map[string]bool)

// GlobalConfig the global config object
var GlobalConfig *config

func ReadGlobalConfig() {
	// Priority of configuration options
	// 1: CLI Parameters
	// 2: environment
	// 3: config.yaml
	// 4: defaults
	config, err := readConfig()
	if err != nil {
		panic(err.Error())
	}

	// Set config object for main package
	GlobalConfig = config
}

var defaultConfig = &config{
	Config:                     ".fishler",
	DockerBasepath:             "./docker",
	RandomConnectionSleepCount: 0,
	Debug:                      false,
	DockerHostname:             "localhost",
	DockerImagename:            "fishler",
	PrivateKeyFilepath:         "/opt/fishler/crypto/id_rsa",
	LogBasepath:                "/var/log/fishler/",
	DockerMemoryLimit:          8,
}

// configInit must be called from the packages' init() func
func ConfigInit(rootCmd *cobra.Command) error {
	cliFlags(rootCmd)
	return bindFlagsAndEnv(rootCmd)
}

// Create private data struct to hold config options.
// `mapstructure` => viper tags
// `struct` => fatih structs tag
// `env` => environment variable name
type config struct {
	DockerMemoryLimit          int      `mapstructure:"docker-memory-limit" structs:"docker-memory-limit" env:"FISHLER_DOCKER_MEMORY_LIMIT"`
	Volumns                    []string `mapstructure:"volumn" structs:"volumn"`
	LogBasepath                string   `mapstructure:"log-basepath" structs:"log-basepath" env:"FISHLER_LOG_BASEPATH"`
	PrivateKeyFilepath         string   `mapstructure:"private-key-filepath" structs:"private-key-filepath" env:"FISHLER_SSH_PRIVATE_KEY_FILEPATH"`
	DockerHostname             string   `mapstructure:"docker-hostname" structs:"docker-hostname" env:"FISHLER_DOCKER_HOSTNAME"`
	DockerImagename            string   `mapstructure:"docker-imagename" structs:"docker-imagename" env:"FISHLER_DOCKER_IMAGENAME"`
	Config                     string   `mapstructure:"config" structs:"config" env:"FISHLER_CONFIG"`
	Port                       int      `mapstructure:"port" structs:"port" env:"FISHLER_PORT"`
	DockerBasepath             string   `mapstructure:"docker-basepath" structs:"docker-basepath" env:"FISHLER_DOCKER_BASEPATH"`
	RandomConnectionSleepCount int      `mapstructure:"random-sleep-count" structs:"random-sleep-count" env:"FISHLER_SSH_CONNECT_SLEEP_COUNT"`
	AccountFilepath            string   `mapstructure:"account-file" structs:"account-file" env:"FISHLER_ACCOUNT_FILE"`
	PasswordFilepath           string   `mapstructure:"password-file" structs:"password-file" env:"FISHLER_PASSWORD_FILE"`
	Account                    string   `mapstructure:"account" structs:"account" env:"FISHLER_ACCOUNT"`
	Password                   string   `mapstructure:"password" structs:"password" env:"FISHLER_PASSWORD"`
	AnyAccount                 bool     `mapstructure:"any-account" structs:"any-account" env:"FISHLER_ANY_ACCOUNT"`
	NoAccount                  bool     `mapstructure:"no-account" structs:"no-account" env:"FISHLER_NO_ACCOUNT"`
	Debug                      bool     `mapstructure:"debug" structs:"debug" env:"FISHLER_DEBUG"`
}

func cliFlags(rootCmd *cobra.Command) {
	// Keep cli parameters in sync with the config struct
	rootCmd.PersistentFlags().StringArrayVarP(&defaultConfig.Volumns, "volumn", "v", []string{}, "")

	rootCmd.PersistentFlags().Int("docker-memory-limit", defaultConfig.DockerMemoryLimit, "The amount of memory (in MB) that each container should get when a user obtains a session")

	rootCmd.PersistentFlags().String("log-basepath", defaultConfig.LogBasepath, "The base filepath where logs will be stored")
	rootCmd.PersistentFlags().String("private-key-filepath", defaultConfig.PrivateKeyFilepath, "The filepath to a private key for the SSH server")
	rootCmd.PersistentFlags().String("docker-hostname", defaultConfig.DockerHostname, "The hostname used in the docker container")
	rootCmd.PersistentFlags().String("docker-imagename", defaultConfig.DockerImagename, "The image user for the docker container")
	rootCmd.PersistentFlags().String("config", defaultConfig.Config, ".fishler.yaml")
	rootCmd.PersistentFlags().BoolP("debug", "d", defaultConfig.Debug, "Output debug information")
	rootCmd.PersistentFlags().Int("port", defaultConfig.Port, "The port to listen on for SSH connections - if not set, will bind to a random high port")
	rootCmd.PersistentFlags().String("docker-basepath", defaultConfig.DockerBasepath, "The path to the docker folder ./docker if run from the root of the project")
	rootCmd.PersistentFlags().Int("random-sleep-count", defaultConfig.RandomConnectionSleepCount, "If non-zero, sleep this at most this many seconds before allowing authentication to continue")

	rootCmd.PersistentFlags().String("account-file", defaultConfig.AccountFilepath, "Exclusive: A file with a list of username/password combinations that are valid for the server (new-line delimited) in the form: username password - quote if space is present in either")
	rootCmd.PersistentFlags().String("password-file", defaultConfig.PasswordFilepath, "Exclusive: A file with a list of passwords that are valid (with any account) for the server (new-line delimited) in the form: password")
	rootCmd.PersistentFlags().String("account", defaultConfig.Account, "Exclusive: An account that is valid (any password) for the server")
	rootCmd.PersistentFlags().String("password", defaultConfig.Password, "Exclusive: A password that is valid (any account) for the server")
	rootCmd.PersistentFlags().Bool("any-account", defaultConfig.AnyAccount, "Any username/password combination will yield in successful authentication to the server")
	rootCmd.PersistentFlags().Bool("no-account", defaultConfig.NoAccount, "No username/pasword combination will every yield in successful authentication to the server")
	rootCmd.MarkFlagsOneRequired("account-file", "password-file", "account", "password", "any-account", "no-account")
	rootCmd.MarkFlagsMutuallyExclusive("account-file", "password-file", "account", "password", "any-account", "no-account")
}

// bindFlagsAndEnv will assign the environment variables to the cli parameters
func bindFlagsAndEnv(rootCmd *cobra.Command) (err error) {
	for _, field := range structs.Fields(&config{}) {
		// Get the struct tag values
		key := field.Tag("structs")
		env := field.Tag("env")

		// Bind cobra flags to viper
		err = viper.BindPFlag(key, rootCmd.PersistentFlags().Lookup(key))
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
func (c *config) Print() {
	cp := *c
	_ = conform.Strings(&cp)
	litter.Dump(cp)
}

// String the config object
// but remove sensitive data
func (c *config) String() string {
	cp := *c
	_ = conform.Strings(&cp)
	return litter.Sdump(cp)
}

// readConfig a helper to read default from a default config object.
func readConfig() (*config, error) {
	// Create a map of the default config
	defaultsAsMap := structs.Map(defaultConfig)

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
	c := &config{}
	err := viper.Unmarshal(c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func Authenticate(username string, password string) bool {
	if GlobalConfig.NoAccount {
		return false
	}

	if GlobalConfig.AnyAccount {
		return true
	}

	if len(passwords) > 0 {
		return passwords[password]
	}

	if len(accounts) > 0 {
		if len(accounts[username]) > 0 {
			return slices.Contains(accounts[username], password)
		}
	}

	return false // fail closed
}

func SetupAuthentication(cmd *cobra.Command) {
	debug, _ := cmd.Flags().GetBool("debug")

	accountFile, _ := cmd.Flags().GetString("account-file")
	passwordFile, _ := cmd.Flags().GetString("password-file")
	account, _ := cmd.Flags().GetString("account")
	password, _ := cmd.Flags().GetString("password")

	if accountFile != "" {
		file, err := os.Open(accountFile)

		// Checks for the error
		if err != nil {
			log.Fatal(err)
		}

		// Closes the file
		defer file.Close()

		reader := csv.NewReader(file)
		records, err := reader.ReadAll()

		if err != nil {
			log.Fatal(err)
		}

		for idx, row := range records {
			if len(row) != 2 {
				fmt.Printf("Bad row %v in %s on line %d\n", row, accountFile, idx)
				continue
			}

			accounts[row[0]] = append(accounts[row[0]], row[1])
		}

		if debug {
			fmt.Printf("Read %d accounts from %s\n", len(accounts), accountFile)

			for account, passwords := range accounts {
				fmt.Printf("Account: %s valid passwords are:\n", account)

				for _, password := range passwords {
					fmt.Printf("\t%s\n", password)
				}
			}
		}
	}

	if passwordFile != "" {
		file, err := os.Open(passwordFile)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			passwords[scanner.Text()] = true
		}
		// check if Scan() finished because of error or because it reached end of file
		err = scanner.Err()

		if err != nil {
			log.Fatal(err)
		}

		if debug {
			fmt.Printf("Read %d passwords from %s\n", len(passwords), passwordFile)
		}
	}

	if account != "" {
		row := strings.Split(account, ",")

		if len(row) != 2 {
			fmt.Printf("Bad format for `--acount` use quotes if a space or comma exists")
		}

		accounts[row[0]] = append(accounts[row[0]], row[1])

		if debug {
			fmt.Printf("Using account `%s` with password `%s`\n", row[0], row[1])
		}
	}

	if password != "" {
		passwords[password] = true

		fmt.Printf("Using password `%s`", password)
	}
}
