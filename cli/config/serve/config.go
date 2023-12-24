package config_serve

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/leebenson/conform"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	configRoot "github.com/archimoebius/fishler/cli/config/root"
)

// Setting is a global config object
var Setting *setting

// initial settings (defaults)
var initial = &setting{
	RandomConnectionSleepCount: 0,
	Banner:                     "OpenSSH_8.8",
	IP:                         "127.0.0.1",
	Port:                       2222,
	DockerHostname:             "localhost",
	CryptoBasepath:             "/opt/fishler/crypto",
	DockerMemoryLimit:          8,
	DockerDiskLimit:            100, // MB
	IdleTimeout:                0 * time.Second,
	MaxTimeout:                 0 * time.Second,
	AccountFilepath:            "",
	PasswordFilepath:           "",
	Account:                    "",
	Password:                   "",
	AnyAccount:                 false,
	NoAccount:                  false,
}

// Create private data struct to hold setting options.
// `mapstructure` => viper tags
// `struct` => fatih structs tag
// `env` => environment variable name
type setting struct {
	Banner                     string        `mapstructure:"banner" structs:"banner" env:"FISHLER_BANNER"`
	DockerMemoryLimit          int           `mapstructure:"docker-memory-limit" structs:"docker-memory-limit" env:"FISHLER_DOCKER_MEMORY_LIMIT"`
	DockerDiskLimit            int64         `mapstructure:"docker-disk-limit" structs:"docker-disk-limit" env:"FISHLER_DOCKER_DISK_LIMIT"`
	IdleTimeout                time.Duration `mapstructure:"ssh-idle-timeout" structs:"ssh-idle-timeout" env:"FISHLER_SSH_IDLE_TIMEOUT"`
	MaxTimeout                 time.Duration `mapstructure:"ssh-max-timeout" structs:"ssh-max-timeout" env:"FISHLER_SSH_MAX_TIMEOUT"`
	Volumns                    []string      `mapstructure:"volumn" structs:"volumn"`
	CryptoBasepath             string        `mapstructure:"crypto-basepath" structs:"crypto-basepath" env:"FISHLER_CRYPTO_BASEPATH"`
	DockerHostname             string        `mapstructure:"docker-hostname" structs:"docker-hostname" env:"FISHLER_DOCKER_HOSTNAME"`
	Port                       int           `mapstructure:"port" structs:"port" env:"FISHLER_PORT"`
	IP                         string        `mapstructure:"ip" structs:"ip" env:"FISHLER_IP"`
	RandomConnectionSleepCount int           `mapstructure:"random-sleep-count" structs:"random-sleep-count" env:"FISHLER_SSH_CONNECT_SLEEP_COUNT"`
	AccountFilepath            string        `mapstructure:"account-file" structs:"account-file" env:"FISHLER_ACCOUNT_FILE"`
	PasswordFilepath           string        `mapstructure:"password-file" structs:"password-file" env:"FISHLER_PASSWORD_FILE"`
	Account                    string        `mapstructure:"account" structs:"account" env:"FISHLER_ACCOUNT"`
	Password                   string        `mapstructure:"password" structs:"password" env:"FISHLER_PASSWORD"`
	AnyAccount                 bool          `mapstructure:"any-account" structs:"any-account" env:"FISHLER_ANY_ACCOUNT"`
	NoAccount                  bool          `mapstructure:"no-account" structs:"no-account" env:"FISHLER_NO_ACCOUNT"`
	accounts                   map[string][]string
	passwords                  map[string]bool
}

func Load() {
	// Priority of configuration options
	// 1: CLI Parameters
	// 2: environment
	// 3: config.yaml
	// 4: defaults

	defaultsAsMap := structs.Map(initial)

	// Set defaults
	for key, value := range defaultsAsMap {
		viper.SetDefault(key, value)
	}
	// Read config from file
	viper.SetConfigType("yaml")
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
	command.PersistentFlags().StringArrayVarP(&initial.Volumns, "volumn", "v", []string{}, "")

	command.PersistentFlags().Int("docker-memory-limit", initial.DockerMemoryLimit, "The amount of memory (in MB) that each container should get when a user obtains a session")
	command.PersistentFlags().Int("docker-disk-limit", int(initial.DockerDiskLimit), "The amount of disk space (in MB) that each container should limit a container to")

	command.PersistentFlags().Duration("ssh-idle-timeout", initial.IdleTimeout, "If a session is idle for this many seconds - terminate it")
	command.PersistentFlags().Duration("ssh-max-timeout", initial.MaxTimeout, "Terminate the session after this many seconds - if 0 no session time limit")

	command.PersistentFlags().String("crypto-basepath", initial.CryptoBasepath, "The basepath to a directory which holds files: id_rsa/id_rsa.pub for the SSH server")
	command.PersistentFlags().String("docker-hostname", initial.DockerHostname, "The hostname used in the docker container")
	command.PersistentFlags().Int("port", initial.Port, "The port to listen on for SSH connections - if not set, will bind to a random high port")
	command.PersistentFlags().String("ip", initial.IP, "The IP to listen on for SSH connections - if not set, will bind to 127.0.0.1")
	command.PersistentFlags().String("banner", initial.Banner, "The banner the SSH server displays")
	command.PersistentFlags().Int("random-sleep-count", initial.RandomConnectionSleepCount, "If non-zero, sleep this at most this many seconds before allowing authentication to continue")

	command.PersistentFlags().String("account-file", initial.AccountFilepath, "Exclusive: A file with a list of username/password combinations that are valid for the server (new-line delimited) in the form: username password - quote if space is present in either")
	command.PersistentFlags().String("password-file", initial.PasswordFilepath, "Exclusive: A file with a list of passwords that are valid (with any account) for the server (new-line delimited) in the form: password")
	command.PersistentFlags().String("account", initial.Account, "Exclusive: An account that is valid (any password) for the server")
	command.PersistentFlags().String("password", initial.Password, "Exclusive: A password that is valid (any account) for the server")
	command.PersistentFlags().Bool("any-account", initial.AnyAccount, "Any username/password combination will yield in successful authentication to the server")
	command.PersistentFlags().Bool("no-account", initial.NoAccount, "No username/pasword combination will every yield in successful authentication to the server")
	command.MarkFlagsOneRequired("account-file", "password-file", "account", "password", "any-account", "no-account")
	command.MarkFlagsMutuallyExclusive("account-file", "password-file", "account", "password", "any-account", "no-account")

	for _, field := range structs.Fields(&setting{}) {
		// Get the struct tag values
		key := field.Tag("structs")

		if key == "" {
			continue
		}

		env := field.Tag("env")

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

func (c *setting) Authenticate(username, password string) bool {
	if c.NoAccount {
		return false
	}

	if c.AnyAccount {
		return true
	}

	if len(c.passwords) > 0 {
		return c.passwords[password]
	}

	if len(c.accounts) > 0 {
		if len(c.accounts[username]) > 0 {
			return slices.Contains(c.accounts[username], password)
		}
	}

	return false // fail closed
}

func (c *setting) SetupAuthentication() {

	if c.AccountFilepath != "" {
		file, err := os.Open(c.AccountFilepath) // #nosec

		// Checks for the error
		if err != nil {
			log.Fatal(err)
		}

		// Closes the file
		defer file.Close()

		reader := csv.NewReader(file)
		records, err := reader.ReadAll()

		if err != nil {
			fmt.Print(err)
			return
		}

		for idx, row := range records {
			if len(row) != 2 {
				fmt.Printf("Bad row %v in %s on line %d\n", row, c.AccountFilepath, idx)
				continue
			}

			c.accounts[row[0]] = append(c.accounts[row[0]], row[1])
		}

		if configRoot.Setting.Debug {
			fmt.Printf("Read %d accounts from %s\n", len(c.accounts), c.AccountFilepath)

			for account, passwords := range c.accounts {
				fmt.Printf("Account: %s valid passwords are:\n", account)

				for _, password := range passwords {
					fmt.Printf("\t%s\n", password)
				}
			}
		}
	}

	if c.PasswordFilepath != "" {
		file, err := os.Open(c.PasswordFilepath) // #nosec
		if err != nil {
			fmt.Print(err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			c.passwords[scanner.Text()] = true
		}
		// check if Scan() finished because of error or because it reached end of file
		err = scanner.Err()

		if err != nil {
			fmt.Print(err)
			return
		}

		if configRoot.Setting.Debug {
			fmt.Printf("Read %d passwords from %s\n", len(c.passwords), c.PasswordFilepath)
		}
	}

	if c.Account != "" {
		row := strings.Split(c.Account, ",")

		if len(row) != 2 {
			fmt.Printf("Bad format for `--acount` use quotes if a space or comma exists")
		}

		c.accounts[row[0]] = append(c.accounts[row[0]], row[1])

		if configRoot.Setting.Debug {
			fmt.Printf("Using account `%s` with password `%s`\n", row[0], row[1])
		}
	}

	if c.Password != "" {
		c.passwords[c.Password] = true

		fmt.Printf("Using password `%s`", c.Password)
	}
}
