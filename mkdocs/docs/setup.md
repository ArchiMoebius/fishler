## Quick Start - Release Binary

0. Download a release

1. copy it to your desired location

2. Jump to 'Configuration'

## Quick Start - I'm'a Developer!

0. `git clone git@github.com:ArchiMoebius/fishler.git && cd fishler`

1. `make`

2. copy ./dist/temp/*flavor* to your desired location

3. Jump to 'Configuration'

## Configuration

You can use a combination of CLI Parameters, environment variables, .fishler.yaml, and default settings to get up and running. The precedence for application of state is as follows:

```bash
	1: CLI Parameters
	2: environment
	3: .fishler.yaml
	4: defaults
```

### Authentication (or not)

The simplest way to deploy Fishler is 'logging' mode where authentication is disabled (you only record IP/Username/Password for attempts).

```use the --no-account flag``` ***In this mode, no docker containers are spawned***

The second simplest method is to deploy it wide open - any username/password combination will spawn a docker container.

```use the --any-account flag```

If you only want a single username/password combination to authenticate use the ```--account``` flag - for example:

```--account root,password``` will allow the root user to be leveraged with a password of password.

If you only want to specify a valid password - use the ```--password``` flag - for example:

```--password password``` will allow any user to be leveraged with a password of password.

If you want a list of valid passwords - use the ```--password-file``` flag - for example:

```--password-file passwords.txt``` where [password.txt](passwords.txt) contains one-password-per-line.

If you want a list of valid username/password combinations - use the ```--account-file``` flag - for example:

```--account-file accounts.csv``` where [accounts.csv](accounts.csv) contains one account in the form <username>,<password> per-line.

If you want to delay (emulate a busy server) on successful authentication - use the ```--random-sleep-count <seconds to wait>```.

### Example Deployments

If one desired to listen on port 2222, allow any username/password combination, bind-mount (Docker syntax) a volumn of data in - they could:

```bash
fishler serve --volumn '/tmp/data/juicy.txt:/juicy.txt:ro' --port 2222 --any-account
```

If one desired to on a random high port, and log - but not authenticate any users (don't spin up a docker container for a session - ever) they could.

```bash
fishler serve --no-account
```

If one desired to listen on port 2222, allow any username/password combination, and emulate a wonky/loaded server (authentication takes a random number of seconds between 1 and 30) - they could:

```bash
fishler serve --port 2222 --any-account --random-sleep-count 30
```